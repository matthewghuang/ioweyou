# I Owe You — Mobile Version Plan

## Goal
A single-page, installable PWA that feels native on mobile: portrait-first, touch-optimized, fully offline-capable via CRDT-based sync, with safe-area awareness for modern phones.

## Done

All six phases are complete:

**Phase 1 — Navigation & Gestures** ✅
Swipe-back from left edge, pull-to-refresh with visual indicator, Toast notifications, BottomSheet confirmations

**Phase 2 — CRDT Offline Sync** ✅
Client-side CRDT engine (mergeState, snapshot, HLC), IndexedDB persistence, push/pull sync service, optimistic UI with pending indicators, auto-sync on reconnect

**Phase 3 — Touch & Interaction Polish** ✅
Swipe-to-reveal action buttons on cards, active state feedback on all interactive elements, momentum scrolling, keyboard avoidance for form inputs

**Phase 4 — Native Feel** ✅
Proper 192x192/512x512 PNG app icons, manifest updated, apple-touch-icon, haptic feedback on 15 buttons

**Phase 5 — Testing** ✅
Playwright Mobile Safari project (iPhone 14, 390x844), offline sync E2E test with context.setOffline(), 34 tests across 5 files

**Phase 6 — Performance** ✅
GroupDetail lazy-loaded as separate chunk (90 kB main / 34 kB lazy), preconnect hint in index.html

## Backend CRDT Foundation (already in place)

The server stores all data as append-only CRDT operations in `crdt_operations` table:

| Component | Role |
|-----------|------|
| `crdt.Operation` | Typed mutation: LWW (scalar fields), RGA insert/delete (list items) |
| `crdt.HLC` | Hybrid Logical Clock — wall time + logical counter, causally ordered |
| `crdt.MergeState` | Deduplicates by `(doc_id, author_id, wall_time, logical)`, resolves conflicts by timestamp |
| `crdt.Snapshot` | Projects merged ops into JSON state |
| `store.SQLiteStateStore` | Append-only persistence, UNIQUE constraint ensures idempotency |
| `POST /api/sync/push` | Accepts operations, deduplicates on insert, broadcasts via WebSocket |
| `POST /api/sync/pull` | Returns ops newer than provided per-author cursors |
| WebSocket `/api/ws` | Real-time push of operations to subscribers |

The pull/push sync protocol is operation-level, not HTTP-response-level. This is the foundation offline sync is built on.

## Phases

### Phase 1 — Navigation & Gestures *(next)*

| Task | Notes |
|------|-------|
| Swipe-back from group detail to group list | `touchstart`/`touchend` on detail view, trigger `onBack` if swipe right > 80px |
| Pull-to-refresh on GroupDetail | Manual `touch` events + spring; re-fetches all data via pull sync |
| Replace `alert()`/`confirm()` with bottom sheet | `confirm('…')` on payment cancel is jarring on mobile |
| Bottom nav or FAB for quick actions | Evaluate: keep top header or add bottom tab bar + FAB for add expense/payment |

### Phase 2 — CRDT Offline Sync *(key phase)*

The app already talks to a CRDT-native backend. Offline isn't about caching HTTP responses — it's about **queuing operations locally and reconciling via the push/pull protocol**.

#### Local Operation Queue

| Task | Notes |
|------|-------|
| Client-side CRDT operation store in IndexedDB | Store `crdt.Operation` objects keyed by group ID. IndexedDB via raw `idb` (no extra deps beyond what's needed) |
| Client-side HLC | A local `HybridLogicalClock` in JS per device session. Each offline-created op gets a timestamp from this clock. Wrapped in a simple class with `now()` and `observe()`. |
| Intercept writes when offline | Instead of `POST /api/groups/{slug}/expenses`, create the CRDT op and: (1) append to local IndexedDB, (2) attempt push via fetch. On network error, leave queued. |
| Online queue flush | On `window.online` + app focus, iterate queued ops per group and push via `POST /api/sync/push` |
| Idempotent push | Server's UNIQUE constraint silently drops duplicates — safe to re-push the same op multiple times |
| Queue persistence | Ops survive tab close / PWA kill. IndexedDB lifetime matches group membership. |

#### Sync Protocol

| Step | Client | Server |
|------|--------|--------|
| 1 | Flush all queued ops: `POST /api/sync/push { operations: [...] }` | Inserts with `INSERT OR IGNORE`; broadcasts accepted ops to group subscribers |
| 2 | Pull newer ops: `POST /api/sync/pull { doc_id?, cursors: { author_id: { wall_time, logical } } }` | Returns ops newer than each cursor, plus updated cursors |
| 3 | Merge: apply `crdt.MergeState` semantics client-side | — |
| 4 | Re-render: snapshot merged state for each group | — |

#### Version Vector (Cursors)

The client maintains a `version_vector` per group — a map of `{ author_id: Timestamp }` that tracks the latest seen op per author. This is:
- Updated on every received WebSocket push
- Updated after every pull response
- Persisted in IndexedDB
- Sent as `cursors` on the next pull

This ensures incremental sync: only ops newer than last seen are transferred.

#### Optimistic UI

| Task | Notes |
|------|-------|
| Apply queued ops to local state immediately | Run `MergeState(localOps, cachedRemoteOps) → Snapshot → render` after each local create |
| Roll back on confirmed server rejection | Remove failed op from queue, re-merge, re-render. Show error toast. |
| Pending indicator | Subtle badge or icon next to items created while offline (e.g. clock icon) |

#### Offline Detection & UX

| Task | Notes |
|------|-------|
| Network status singleton | `navigator.onLine` + `online`/`offline` events → Svelte store `$networkOnline` |
| Offline banner | Fixed bottom bar: "You're offline — changes will sync when connected" with pending count |
| Form submission while offline | Same flow as online (creates CRDT op), but banner shows + op stays queued |
| Reconnect flush on app resume | On `online` event + document `visibilitychange` to `visible`, run flush loop |

#### Cache-First Reads When Offline

| Task | Notes |
|------|-------|
| Group list from IndexedDB | On page load, snapshot from local ops immediately; then pull in background |
| Expense/Payment/Balance views | Same: local snapshot first, then reconcile |
| Stale indicator | Timestamp of last sync; show "Last updated X min ago" |

#### Conflict Resolution

Already handled by the CRDT layer:
- **LWW fields** (group name, amounts): highest `(wall_time, logical)` wins. Same-author timestamps are strictly monotonic (HLC guarantees), so last-write-wins within one device.
- **RGA lists** (expense items, payment splits): inserts are ordered by `prev_item_id` references; deletes are tombstoned.
- **Concurrent edits from different devices**: HLC timestamps + author tiebreaker produce a deterministic total order.

No additional conflict UI needed — the CRDT algebra converges automatically.

---

### Phase 3 — Touch & Interaction Polish
| Task | Notes |
|------|-------|
| Swipe-to-delete on expense/payment cards | Reveal "Delete" / "Cancel" action behind a swipe |
| Long-press context menu on group cards | Quick actions: Share, Leave, Copy invite |
| Active state feedback on all taps | Verify `:active` on mobile Chrome/Safari |
| Momentum-scrolling lists | `-webkit-overflow-scrolling: touch` on scrollable regions |
| Keyboard avoidance on form inputs | Ensure form scrolls into view when keyboard opens (iOS especially) |

### Phase 4 — Native Feel
| Task | Notes |
|------|-------|
| Splash screen on PWA launch | `apple-touch-startup-image` or rely on `theme_color` + `background_color` |
| Status bar styling | Already `black-translucent` — verify text readable over dark bg |
| Haptic feedback on actions | `navigator.vibrate` (Android), Haptic Feedback API (iOS Safari 16.4+) |
| App icon variants | Generate proper 192x192 / 512x512 PNG icons; SVG data URI won't show on all launchers |
| `prefers-color-scheme` light theme | Optional — current dark-only is fine for PWA but document as future option |

### Phase 5 — Testing
| Task | Notes |
|------|-------|
| Playwright tests at mobile viewport | Add `projects` for `iPhone 14` device in config |
| E2E: create group, add expense, record payment | Run at `390x844` viewport |
| E2E: offline create → go online → reconcile | Mock offline with `page.route('**/api/**', route => route.abort())`, then re-enable |
| E2E: PWA install prompt | Manual (install promotion in Chrome) |
| Touch: swipe, pull-to-refresh, tap targets | Manual QA on real device |
| CRDT sync correctness | Verify ops pushed from offline client produce same state as online-only, even with concurrent edits |

### Phase 6 — Performance
| Task | Notes |
|------|-------|
| Lazy-load page components | Code-split GroupDetail (largest chunk) |
| Preconnect to API origin | `<link rel="preconnect">` in index.html |
| Critical CSS inline | Extract above-fold styles for instant render |
| Image optimization | Currently no raster images; keep it that way |
| Lighthouse audit target | Score ≥90 on PWA, Performance, Best Practices |

## Architecture Decisions

### State Management
Keep Svelte stores + IndexedDB for offline op queue and cached state. No heavier state lib needed.

### Client-Side CRDT
Implement a minimal JS module that mirrors the Go CRDT logic:
- `mergeState(local: Operation[], remote: Operation[]) → Operation[]` — dedup + conflict resolution
- `snapshot(ops: Operation[]) → Object` — project merged ops into renderable state
- `HLC` — local hybrid logical clock

Alternatively, call `Snapshot` via a lightweight WASM compilation of the Go CRDT package, but a pure-JS port of the ~200 lines of merge logic is simpler and avoids WASM build complexity.

### Routing
Keep `history.pushState` + `currentPage` store. No router library — the page count doesn't justify it.

### API Interaction for Offline Writes
No HTTP caching layer for writes. Instead:
- **Writes** → create `crdt.Operation`, store in IndexedDB, attempt push. On failure, leave queued.
- **Reads** → snapshot from locally stored ops + pulled remote ops. No SWR/stale-while-revalidate; the merge is the source of truth.

This is the correct architectural choice because the server already treats operations as the source of truth — HTTP response caching would bypass the CRDT merge layer and produce stale or inconsistent state.

### Offline Storage
IndexedDB via raw `idb` (lightweight wrapper, ~2KB) or direct IndexedDB API. Not localStorage — synchronous, 5–10MB cap, no structured storage.

### Data Model in IndexedDB
| Store | Key | Value |
|-------|-----|-------|
| `ops` | `[groupId, authorId, wallTime, logical]` | `crdt.Operation` |
| `version_vectors` | `groupId` | `{ [authorId]: Timestamp }` |
| `members` | `groupId` | Cached member list (not CRDT, from auth) |

