# Adversarial Review Report

**Project**: I Owe You (ioweyou)
**Reviewer**: adversarial-review
**Date**: 2026-07-30

---

## CRITICAL

### 1. Auth Token Leaked in WebSocket URL Through Server Logs
- **File**: `internal/sync/websocket.go:L35` + `internal/api/router.go:L18`
- **Issue**: The WebSocket auth token is passed as a query parameter (`?token=<cookie_token>`). The chi `Logger` middleware logs every request URL including query parameters. Every WebSocket upgrade attempt logs the full URL with the token. Additionally, the `RealIP` middleware combined with logging means tokens are written to stdout/stderr wherever the server is deployed. On Fly.io or Docker, these logs persist and are accessible to platform operators.
- **Impact**: Anyone with access to server logs (hosting platform, CI logs, log aggregator) can read active auth tokens. This bypasses all group authentication.
- **Fix**: Move WebSocket authentication to a custom header or the first message after upgrade (send token as a `{"type":"auth","token":"..."}` JSON message after the WebSocket handshake). Remove token from the URL query string. Alternatively, strip query params from the chi logger output.

### 2. Auth Token Leaked in Browser History
- **File**: `frontend/src/lib/websocket.js:L19`
- **Issue**: The WebSocket URL is constructed with the token in the query string. If the WebSocket connection triggers a page navigation or if any error page includes this URL, the token could be saved in browser history. While WebSocket URLs aren't typically stored in history, any `console.log` output containing the URL is visible in devtools.
- **Impact**: Token exposure to anyone with access to the browser's developer tools, bookmarks, or history.
- **Fix**: Same as above — authenticate via the first WebSocket message.

### 3. No Rate Limiting on Any Endpoint
- **File**: `internal/api/router.go` (entire file), `internal/api/groups.go`
- **Issue**: There is no rate limiting on any API endpoint. An attacker can:
  - Brute-force group secrets via `POST /api/groups/join` (unlimited attempts)
  - Flood the server with operations via `POST /api/sync/push`
  - Create unlimited groups via `POST /api/groups`
  - Exhaust SQLite connections and disk space
- **Impact**: Complete denial of service via resource exhaustion. Secret brute-forcing compromises group confidentiality.
- **Fix**: Implement per-IP and per-token rate limiting middleware. Add exponential backoff on failed join attempts. Limit group creation per IP.

### 4. Group Secret Brute-Force via Join Endpoint
- **File**: `internal/api/groups.go:L121-L132`
- **Issue**: `POST /api/groups/join` is public and accepts unlimited attempts. The secret can be any string with no complexity requirements. The endpoint returns distinct errors for "group not found" (404) vs "invalid secret" (401), enabling attackers to first enumerate valid groups, then brute-force the secret.
- **Impact**: Complete compromise of any group whose secret is weak (short, common, or brute-forced).
- **Fix**: Add rate limiting per-IP on the join endpoint. Require minimum secret length (e.g., 8 characters). Consider a proof-of-work or CAPTCHA for join attempts. Return a generic error regardless of whether the group or secret was wrong.

### 5. Group Slug Collision Space Is Only 65536 Possibilities
- **File**: `internal/auth/auth.go:L94-L107`
- **Issue**: `GenerateSlug` appends only 4 random hex characters (2 bytes = 16 bits). With 65536 possible suffixes, the birthday paradox means collisions become likely at around 300 slugs with the same base name. The UNIQUE constraint on the `slug` column will catch the collision, but the response is a 500 error with the raw database error leaked to the client.
- **Impact**: Group creation fails with a database error that leaks schema internals. Users cannot create groups with popular names.
- **Fix**: Increase random suffix to at least 8 bytes (16 hex chars). Loop on UNIQUE constraint violation and retry with a new suffix (up to N attempts).

### 6. JSON Value Query Replay Attack — No Group Boundary Enforcement on Push
- **File**: `internal/sync/handler.go:L130-L157`
- **Issue**: The push handler determines group membership by parsing the operation's `group_id` / `name` field values BEFORE persisting. A malicious group member could craft an operation that claims to belong to their group but actually targets a document in a group they don't have access to. The `group_id` value in the operation JSON is untrusted.
- **Impact**: A malicious group member could potentially push operations into another group's documents if they can guess or discover other group's document IDs.
- **Mitigation**: The approach works for new documents where the group_id field is set in the same batch. For existing documents, `resolveDocGroup` queries the DB for the authoritative group_id. But an attacker could know a victim's document UUID (e.g., from a push notification broadcast).
- **Fix**: Always verify against the authoritative DB group_id for existing documents. Reject operations where the claimed group_id doesn't match the DB record. For new documents, verify the target group membership directly.

### 7. No Content Security Policy (CSP) Header
- **File**: `internal/api/router.go` (missing middleware)
- **Issue**: No CSP headers are set anywhere in the response chain. While Svelte escapes HTML by default, any XSS vulnerability (e.g., in group names, descriptions, or future features) would be trivially exploitable. The `Access-Control-Allow-Origin: *` header combined with no CSP makes data exfiltration easy.
- **Impact**: Any XSS vulnerability becomes immediately weaponizable for data theft, session hijacking, or phishing.
- **Fix**: Set a strict CSP header via middleware: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self' ws: wss:; img-src 'self' data:;`. Mitigates XSS even if HTML injection occurs.

### 8. Offline/Undo Bug: `created` Variable Out of Scope in Toast Callback
- **File**: `frontend/src/pages/GroupDetail.svelte:L402-L409`
- **Issue**: In `handleCreateExpense`, the `created` variable is declared with `const` inside the `if (get(online))` block, but the `showToast` call and its `onClick` callback are outside that block. When the "Undo" button is clicked, `created` is `undefined`, causing a `ReferenceError` error that crashes the callback silently or throws a runtime error. The user sees no feedback when trying to undo.
- **Impact**: "Undo" button after creating an expense is completely broken — clicking it does nothing (or throws a silent error). Users who accidentally create an expense cannot undo it.
- **Fix**: Declare `let created;` before the `if` block, assign inside both branches, and ensure it's available in the `onClick` closure.

### 9. All API Responses Cached in PWA Service Worker Without Auth Awareness
- **File**: `frontend/vite.config.js:L37-L46`
- **Issue**: The Workbox runtime caching configuration caches ALL `/api/` responses (status 200) for up to 1 hour with 50 entries. The cache is keyed by URL only, not by auth token. If a user logs out and a different user accesses the same PWA on the same device (or if the token changes), the cached responses from the previous session could be served, exposing group data across sessions.
- **Impact**: Cross-session data leakage. Old group data served after logout or token refresh.
- **Fix**: Add `fetchOptions: { credentials: 'same-origin' }` and consider using a network-only strategy for sensitive endpoints. Alternatively, add the token hash to the cache key.

### 10. CORS Policy Allows Any Origin to Read API Responses
- **File**: `internal/api/router.go:L32`
- **Issue**: `Access-Control-Allow-Origin: *` with `Access-Control-Allow-Headers: X-Group-Token` means any website can make authenticated requests to the API if it knows the user's token. While `X-Group-Token` is a custom header (protected by CORS preflight), once a token is obtained via XSS or log leakage, any site can read the victim's data.
- **Impact**: Exacerbates all other token-leakage issues. Reduces security from "need token" to "need token + any origin".
- **Fix**: Restrict `Access-Control-Allow-Origin` to the specific frontend origin(s) in production. Use `Access-Control-Allow-Credentials: true` with specific origins for cookie-based auth if migrated.

---

## HIGH

### 11. Floating-Point Arithmetic for Money Amounts
- **File**: `internal/api/balances.go:L109-L131`, `internal/api/expenses.go:L63`, `frontend/src/pages/GroupDetail.svelte` (throughout)
- **Issue**: All monetary amounts are stored and computed as `float64`. Floating-point arithmetic introduces rounding errors that accumulate over many transactions. While `math.Round(amt*100)/100` is used for display, the internal `balances` map accumulates errors. After many expenses, the total might be off by a few cents. The "Remaining balance" breakdown item absorbs rounding, but the net balance computation itself is inexact.
- **Impact**: Balance computation can have cent-level errors that accumulate over time. Users may see non-zero balances that should be zero, or vice versa.
- **Fix**: Use integer arithmetic for cents (store amounts as `int64` representing cents). Convert to/from dollars only for display.

### 12. HLC Not Thread-Safe on Frontend (JS Single-Threaded but Async Race)
- **File**: `frontend/src/lib/crdt.js:L97-L109`
- **Issue**: The frontend HLC `now()` method has no locking. While JavaScript is single-threaded, `async`/`await` can interleave operations. If two async functions call `hlc.now()` between the same clock tick, they can get identical timestamps (same `wall_time` and `logical`). Since the frontend uses these timestamps for CRDT operations, collisions can lead to data loss via the `INSERT OR IGNORE` deduplication on the server.
- **Impact**: Duplicate timestamps cause operations to be silently dropped. This can manifest as missing expense updates or payments that never sync.
- **Fix**: Make the HLC `now()` method synchronous-only and never yield between reading and incrementing. Document that it must not be called from concurrent async contexts without external serialization.

### 13. Membership Check Bypass in ListExpenses/ListPayments/GetBalances
- **File**: `internal/api/expenses.go:L84-L87`, `internal/api/payments.go:L76-L79`, `internal/api/balances.go:L33-L36`
- **Issue**: The `value` field is queried with `string(mustMarshal(gid))`, which adds JSON quotes around the UUID. But the stored `group_id` value in the `crdt_operations` table is also JSON-encoded via `mustMarshal(gid)` in the create paths. This means the query correctly matches. However, if a malicious client managed to store a `group_id` field with a different JSON encoding (e.g., a raw string without quotes via the push endpoint), the query would miss it, creating a blind spot.
- **Impact**: Weak invariant; any client that pushes operations with a non-JSON-encoded `group_id` field could bypass the membership check for listing.
- **Fix**: After fetching doc_ids, independently verify that each document's resolved group (via `resolveDocGroup`) matches the requested group, rather than relying on the raw `value` equality.

### 14. IndexedDB Compound Key Range Query May Produce Wrong Results
- **File**: `frontend/src/lib/db.js:L76-L79`
- **Issue**: `getOps(groupSlug)` uses `IDBKeyRange.bound([groupSlug], [groupSlug + '\uffff'])` to query the `ops` object store whose keys are arrays `[doc_id, author_id, wall_time, logical]`. While IndexedDB's compound key comparison handles variable-length arrays, the upper bound `[groupSlug + '\uffff']` creates a single-element array, which matches all keys whose first element starts with `groupSlug + '\uffff'`. But this would miss keys where `doc_id` is _exactly_ `groupSlug` but the second element (`author_id`) is different. The range `[groupSlug]` to `[groupSlug + '\uffff']` actually works because `[slug]` < `[slug, any_author, ...]` and `[slug + '\uffff']` is a boundary that... hmm.
  
  Actually, this is correct: `IDBKeyRange.bound([prefix], [prefix + '\uffff'])` works for compound array keys because in IndexedDB, `[prefix]` (single element) is less than `[prefix, anything, ...]`, and `[prefix + '\uffff']` acts as an upper bound for the first element. But there's a subtle issue: if `doc_id` is exactly `groupSlug + 'x'` where `'x' > '\uffff'`, it won't be matched. For UUID-length strings, this is not an issue since UUID chars are hex (0-9, a-f) < '\uffff'.
  
  However, `clearOps` has the same pattern and uses cursor-based deletion that could fail silently if a cursor error occurs.
- **Impact**: Low risk, but if `clearOps` fails partially, stale operations accumulate in IndexedDB.
- **Fix**: Add error handling on the cursor's `delete()` operation.

### 15. No Input Length Validation on User or Group Names
- **File**: `internal/api/groups.go:L92-L99`, `internal/auth/auth.go:L92-L107`
- **Issue**: Group names, creator names, and member names have no length limits. An attacker can create groups with names exceeding database limits (SQLite's `TEXT` limit is 1 billion bytes, but in practice much lower). Server memory and bandwidth are consumed storing and transmitting arbitrarily long names.
- **Impact**: Partial denial of service; slow responses for all group members due to oversized payloads; database bloat.
- **Fix**: Add maximum length constraints (e.g., name ≤ 100 chars, group name ≤ 200 chars) on both frontend and backend.

### 16. Leaving Group Does Not Remove CRDT Data
- **File**: `internal/api/groups.go:L238-L259`
- **Issue**: When a member leaves a group, only their `members` row is deleted. All CRDT operations (expenses, payments) the user authored remain in the database. The user's personal financial data persists indefinitely with no cleanup mechanism.
- **Impact**: Privacy concern — leaving a group does not delete your data. The group creator has no way to purge a former member's data.
- **Fix**: Add an optional "delete my data" flag to leave group. At minimum, soft-delete the member's operations or document the behavior in the UI.

### 17. No Database Connection Pool Limits for SQLite
- **File**: `internal/store/sqlite.go:L18-L26`
- **Issue**: `sql.Open` uses Go's default settings: `SetMaxOpenConns(0)` (unlimited) and `SetMaxIdleConns(2)`. For SQLite with WAL mode, too many concurrent writers still cause `SQLITE_BUSY` errors. Each HTTP request or WebSocket handler can open a connection, and with many concurrent users, the DB can become locked.
- **Impact**: Random "database is locked" errors under moderate concurrent load.
- **Fix**: Set `db.SetMaxOpenConns(1)` (SQLite only supports one writer at a time anyway) and use a reasonable idle pool size. Use retry logic for `SQLITE_BUSY` errors.

### 18. `resolveDocGroup` Has an Injection-Like Pattern (Unsafe SQL)
- **File**: `internal/sync/handler.go:L303-L329`
- **Issue**: `resolveDocGroup` queries the `crdt_operations` table using parameterized queries, so SQL injection is not directly exploitable. However, the function determines group ownership by checking if a document has a `name` field (group doc) vs a `group_id` field (child doc). An attacker could push an operation with both `name` and `group_id` fields, confusing the resolution logic.
- **Impact**: Potential group boundary confusion if documents have ambiguous fields.
- **Fix**: Use a deterministic heuristic — group docs have the `name` field set and typically no `group_id` field. Validate at creation time.

### 19. Workbox API Cache Has No Auth-Token-Based Invalidation
- **File**: `frontend/vite.config.js:L37-L46`
- **Issue**: The PWA service worker caches API responses using `NetworkFirst` strategy with a max age of 1 hour. The cache key is the URL only. If a user changes groups, gets new tokens, or logs out, stale data from old sessions may be served. When the token is revoked and a new one issued (via re-join), the old cached data is still served from cache until TTL expires.
- **Impact**: Stale group data visible after token changes or logout.
- **Fix**: Set `cacheName` per-token-hash, or use `NetworkOnly` for authenticated API endpoints and only cache public responses.

### 20. Notification Broadcast Operations Leak to Group Members Who Left
- **File**: `internal/sync/websocket.go:L150-L160`, `internal/sync/broadcaster.go:L56-L76`
- **Issue**: When a WebSocket client is subscribed to a group, they receive broadcast notifications for every change. If a member's `members` row is deleted (they left the group) but their WebSocket connection remains open, they continue receiving broadcasts until the WebSocket disconnects. The `readPump` doesn't re-check membership after initial subscription.
- **Impact**: A member who left the group continues receiving real-time updates about expenses and payments until their WebSocket reconnects (up to 5 minutes of reconnection delay on the frontend).
- **Fix**: On membership change (leave group), server-side disconnect all WebSocket connections for that member. Or re-verify membership before each broadcast send.

---

## MEDIUM

### 21. Unauthenticated Group Info Exposes Member Count and Names
- **File**: `internal/api/groups.go:L152-L167`
- **Issue**: `GET /api/groups/{slug}/info` is publicly accessible and returns member names and count. Anyone who knows a group slug can enumerate members.
- **Impact**: Privacy issue — group membership can be enumerated without authentication.
- **Fix**: Either require authentication or limit the returned data to just the group name and member count (not names). Consider requiring a token to view member details.

### 22. Frontend HLC Initialization on Every Component Mount
- **File**: `frontend/src/pages/GroupDetail.svelte:L171`
- **Issue**: `let hlc = new HLC()` creates a new HLC on each component mount. If the component is remounted (e.g., navigating away and back), the HLC starts from the current time, potentially generating timestamps older than previously synced operations. This can cause new operations to be rejected as duplicates by the server.
- **Impact**: Operations created after re-mounting the component may have timestamps that collide with or precede existing operations, causing them to be silently dropped.
- **Fix**: Store the HLC state in localStorage or IndexedDB so it persists across component remounts. Initialize from the stored state on mount.

### 23. `getOps` Does Not Filter by DocID — Loads All Group Ops
- **File**: `frontend/src/lib/db.js:L72-L82`
- **Issue**: `getOps(groupSlug)` returns ALL operations for a group, filtered only by `doc_id` prefix. The caller then filters in JS memory. For groups with thousands of operations, this loads the entire operation history into memory on every sync.
- **Impact**: Performance degradation for large groups. Memory pressure on mobile devices.
- **Fix**: Add a `docID` parameter to `getOps` and use an IndexedDB cursor that only iterates matching documents. Add pagination.

### 24. Toast Store Has No Maximum Size Limit
- **File**: `frontend/src/lib/toastStore.js`
- **Issue**: The `toasts` writable store grows unbounded. If a rapid sequence of toasts is triggered (e.g., many push notifications arriving simultaneously), the toasts array grows without limit, consuming memory and rendering many DOM nodes.
- **Impact**: Memory leak under rapid notification scenarios. UI clutter.
- **Fix**: Limit the toast queue to a reasonable maximum (e.g., 5). Drop oldest when limit is exceeded.

### 25. Secret Passed in Plaintext in All API Requests
- **File**: `internal/api/groups.go:L105`, `internal/api/groups.go:L123`
- **Issue**: The group secret is sent in plaintext as a JSON field in POST bodies. While `fly.toml` has `force_https = true`, self-hosted deployments may not enforce HTTPS. The secret is also visible in browser devtools network tab.
- **Impact**: Secret exposure to anyone with network access or browser devtools access.
- **Fix**: Document that HTTPS is required. Consider using a password hashing scheme client-side before sending (though this complicates things). At minimum, warn in the UI.

### 26. No Audit Trail for Sensitive Operations
- **File**: All API handlers
- **Issue**: There is no logging of who created/updated/deleted what. The CRDT operations store changes but there's no queryable audit trail associating operations with member identities for non-repudiation.
- **Impact**: Cannot determine which member deleted an expense or confirmed a payment after the fact.
- **Fix**: Log member_id + action + doc_id + timestamp to an audit table or structured log.

### 27. Broadcast Channel Capacity Drop Silently Loses Updates
- **File**: `internal/sync/broadcaster.go:L68-L76`
- **Issue**: The `Broadcast` method sends to each subscriber's channel with a `default` case. If the channel is full (buffer of 256), the message is silently dropped. For fast-updating groups, members can miss expense creations, payment confirmations, or balance changes without any indication.
- **Impact**: Users miss real-time updates. The WebSocket becomes unreliable for critical data.
- **Fix**: Increase channel buffer or implement a slow-client disconnection strategy instead of silent drop. Log dropped messages.

### 28. No Error Handling If `group_id` Field Is Missing or Invalid Type
- **File**: `internal/api/expenses.go:L96-L98`, `internal/api/balances.go:L81`
- **Issue**: `state["group_id"].(string)` performs a type assertion that returns `""` if the field is missing or not a string. The subsequent `isGroupMember` check then uses an empty string, which returns `false`, making the document invisible. This is correct behavior for security, but the error path returns "not a member" (403) instead of a more descriptive error.
- **Impact**: Data corruption renders documents permanently inaccessible with a misleading error message.
- **Fix**: Validate the `group_id` field type on snapshot/read. If corrupt, return a 500 error indicating data corruption rather than a 403 suggesting unauthorized access.

### 29. `DELETE /api/payments/{id}` Does Not Validate Token Is From Payer
- **File**: `internal/api/payments.go:L430-L467`
- **Issue**: The `CancelPayment` handler checks that the requester is either the `from_user` or `to_user`. This is correct. However, the error message leaks which user is which: if the caller is not either, they get "only the sender or recipient can cancel". This reveals the parties involved.
- **Impact**: Minor information disclosure of payment participants.
- **Fix**: Return a generic "forbidden" message without specifying who the parties are.

### 30. WebSocket `resolveDocGroups` Can Trigger Error Log Spam
- **File**: `internal/sync/websocket.go:L189-L206`
- **Issue**: `resolveDocGroups` calls `store.GetLatestState(op.DocID)` for every unique doc_id in a push batch. If a document doesn't exist, the error is silently ignored. But the function continues. For large push batches, this could mean many wasted DB queries.
- **Impact**: Performance issue for large push batches; many unnecessary `GetLatestState` calls for non-existent docs.
- **Fix**: Cache state lookups or batch-resolve doc groups with a single SQL query.

---

## LOW

### 31. Group Name Slug Generation Strips Non-ASCII but Accepts Unicode Letters
- **File**: `internal/auth/auth.go:L94-L107`
- **Issue**: `GenerateSlug` passes Unicode letters through (CJK, Cyrillic, Arabic, etc.) but replaces spaces with hyphens. This creates visually confusable slugs (homograph attacks). For example, a Cyrillic `а` (U+0430) looks identical to Latin `a` (U+0061). A group could create a slug impersonating another group.
- **Impact**: Phishing — a user could be tricked into joining a lookalike group.
- **Fix**: Restrict slug characters to `[a-z0-9-]`. Apply NFD normalization and strip combining characters.

### 32. QR Code and Share Link Include Only the Slug
- **File**: `frontend/src/pages/Groups.svelte:L63`, `frontend/src/pages/Landing.svelte:L28`
- **Issue**: The invite link is constructed as `${window.location.origin}/group/${slug}`. This only includes the slug. A user who receives this link must know the group secret to join (sent separately). However, the link itself leaks the slug, which enables enumeration.
- **Impact**: Trivial information disclosure of group existence.
- **Fix**: This is by design for shareable links. Consider using unguessable invite codes instead of slugs, or make slugs random-only (not based on group name).

### 33. `syncAll()` Iterates Groups Sequentially, Not Concurrently
- **File**: `frontend/src/lib/sync.js:L92-L106`
- **Issue**: `syncAll()` uses `for...of` with `await`, syncing groups one at a time. For users with many groups, this is slow. The comment "eslint-disable-next-line no-await-in-loop" indicates awareness.
- **Impact**: Slow sync for users with many groups.
- **Fix**: Use `Promise.allSettled()` to sync groups in parallel.

### 34. `loadGen` Counter May Wrap After 2^53
- **File**: `frontend/src/pages/GroupDetail.svelte:L170`
- **Issue**: `loadGen` is a JS `number` (64-bit float). After 2^53 increments, integer precision is lost. Realistically unreachable but worth noting.
- **Impact**: Theoretical stale response issue after years of continuous use.
- **Fix**: Use a monotonic counter or UUID per load.

### 35. Error Object Includes Status Code but Not Status Text
- **File**: `frontend/src/lib/api.js:L75`
- **Issue**: `error.status = res.status` sets only the numeric status code. The status text (e.g., "Forbidden") is not included. All error messages show `Request failed (403)` without context.
- **Impact**: Reduced debuggability for users.
- **Fix**: Include `res.statusText` in the error object or message.

### 36. Missing `package-lock.json` in Docker Build Cache Invalidation
- **File**: `Dockerfile:L13-L14`
- **Issue**: Only `go.mod` and `go.sum` are copied before `go mod download`. The frontend stage copies `package.json` and `bun.lock`. But the Dockerfile doesn't copy these before `bun install`, so layers are cached only when the lockfiles change. This is actually correct for the frontend stage.
  
  However, the `COPY . .` on line 16 copies everything before building the Go binary, invalidating the cache even if only frontend files changed.
- **Impact**: Longer CI build times when frontend files change (the Go layer cache is busted).
- **Fix**: Split the Go build into two COPY steps: first `go.mod`/`go.sum` for dependency caching, then the source. Actually this is already done for Go. But the issue is that `COPY . .` on line 16 copies frontend files too, which changes the hash. The frontend is already built in a separate stage, so the Go stage doesn't need the frontend files. Use a `.dockerignore` to exclude `frontend/` from the Go builder context, or use separate COPY commands.

### 37. No Health Check Endpoint
- **File**: `internal/api/router.go`, `fly.toml`
- **Issue**: There is no `/health` or `/ready` endpoint. Fly.io has no health check configured (no `[checks]` section). Docker has no `HEALTHCHECK` instruction. The server could be accepting traffic while the database is still initializing or in a degraded state.
- **Impact**: Users see 500 errors during startup or DB degradation. Traffic is routed to unhealthy instances on Fly.io.
- **Fix**: Add a `/api/health` endpoint that checks DB connectivity. Configure Fly.io `[checks]` and Docker `HEALTHCHECK`.

### 38. `defer rows.Close()` Called but Error Not Checked
- **File**: `internal/api/expenses.go:L93`, `internal/api/payments.go:L85`, `internal/api/balances.go:L49`, `internal/store/sqlite.go:L64`
- **Issue**: `defer rows.Close()` is called after checking `rows.Err()`, which is correct. However, the error from `rows.Close()` itself is never checked. If closing the rows fails (e.g., because of an unread result set), the error is silently lost.
- **Impact**: Silent resource cleanup failures.
- **Fix**: This is a minor Go best practice. The deferred Close error could be logged.

### 39. CI Builds and Tests Don't Run Go Tests
- **File**: `.github/workflows/ci.yml`
- **Issue**: The CI workflow builds the Go binary but never runs `go test ./...`. Only Playwright E2E tests are run. Any Go unit test failures go undetected.
- **Impact**: Go test regressions are not caught in CI.
- **Fix**: Add a `go test ./...` step.

### 40. Docker Container Runs as Non-Root But Data Directory Permissions May Be Wrong
- **File**: `Dockerfile:L30-L33`
- **Issue**: The `appuser` is created with `adduser -D -g '' appuser`. The `chown appuser:appuser /data` sets ownership. However, `docker-compose.yml` creates a named volume `ioweyou-data` mounted at `/data`. On first run, the volume is empty and owned by root. The Dockerfile's `chown` happens during build time on the `/data` directory in the image, but the mounted volume replaces `/data`. When the volume is first created, it's owned by root, so `appuser` can't write to it.
- **Fix**: Add an entrypoint script that ensures `/data` is writable at startup, or use a Docker volume with user namespace mapping.

### 41. `ts` in `created_at` Uses Milliseconds While HLC Uses Nanoseconds
- **File**: `internal/api/expenses.go:L85`, `internal/api/payments.go:L164`
- **Issue**: `created_at` stores `time.Now().UnixMilli()` (millisecond epoch), while the HLC `wall_time` uses nanosecond precision (`UnixNano`). This inconsistency means `created_at` cannot be directly compared with timestamps for ordering.
- **Impact**: Cannot sort by HLC time and created_at interchangeably. Minor confusion during debugging.
- **Fix**: Use consistent time units. Either store everything in milliseconds (simpler for JS consumption) or nanoseconds.

### 42. `crypto.randomUUID()` Not Available in All Browsers
- **File**: `frontend/src/pages/GroupDetail.svelte:L341`, L368, L414
- **Issue**: Uses `crypto.randomUUID()` for generating document and item IDs. This API is available in secure contexts only (HTTPS) and is not supported in older browsers (requires Chrome 92+, Safari 15.4+). In non-secure contexts (HTTP), `crypto.randomUUID()` throws.
- **Impact**: App crashes on HTTP connections or older browsers.
- **Fix**: Use a polyfill or fallback to `crypto.getRandomValues()` with a manual UUID formatter.

### 43. No Docker Healthcheck Instruction
- **File**: `Dockerfile`
- **Issue**: No `HEALTHCHECK` instruction. Docker and orchestrators cannot detect when the application process is running but not serving requests (e.g., during a DB migration or connection failure).
- **Impact**: Unhealthy containers are not restarted automatically.
- **Fix**: Add `HEALTHCHECK --interval=30s --timeout=3s CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1`.

### 44. Docker Build Copies All Files Including `frontend/node_modules`
- **File**: `Dockerfile:L16`
- **Issue**: `COPY . .` copies the entire project, including `frontend/node_modules` if present, into the Go builder stage. The `.dockerignore` may not exclude `node_modules`.
- **Impact**: Large build context, slower Docker builds.
- **Fix**: Confirm the `.dockerignore` includes `frontend/node_modules`. Currently the `.dockerignore` has 446 bytes.
