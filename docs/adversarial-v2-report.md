# Adversarial Review Report — v2

**Project**: I Owe You (ioweyou)
**Reviewer**: adversarial-v2
**Date**: 2026-07-31
**Scope**: Recent fixes in SPA serving, Content-Length, reloading guard, and CSP headers

---

## CRITICAL

### 1. Path Traversal in SPA Fallback File Serving
- **File**: `internal/api/router.go:L116-L122`
- **Issue**: The `r.NotFound` handler constructs file paths by joining `absDir` with `r.URL.Path` using `filepath.Join`. Because `filepath.Join` resolves `..` segments, and `r.URL.Path` is taken from the raw HTTP request without sanitization, an attacker can escape the intended `absDir` and read arbitrary files on the server filesystem.

  ```go
  filePath := filepath.Join(absDir, r.URL.Path)
  if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
      http.ServeFile(w, r, filePath)
      return
  }
  ```

  **Exploit example**:
  ```
  GET /../../../etc/passwd HTTP/1.1
  ```
  → `filepath.Join("/app/frontend/dist", "/../../../etc/passwd")` → `/etc/passwd` (path cleaned)
  → `os.Stat("/etc/passwd")` reports a regular file
  → `http.ServeFile(w, r, "/etc/passwd")` serves the file to the attacker.

  `http.ServeFile` protects against symlink escape relative to the *file's own parent directory*, not relative to `absDir`. The `os.Stat` check creates a TOCTOU race window but does not prevent traversal.

  An attacker can read any file accessible to the server process: application config files, database files, environment files, private keys, or source code.

- **Impact**: Full read capability for any file the server process can access. For projects running with Docker and a mounted data volume, an attacker could read the SQLite database containing all group data, expense details, and auth tokens.
- **Fix**: Do NOT use `r.URL.Path` directly in file path construction. Options:
  1. Use `http.FileServer` + `http.Dir` (which has built-in traversal protection via `EvalSymlinks` prefix check) for all file serving, not just `/assets/*`.
  2. Strip and reject `..` from `r.URL.Path` before constructing the file path.
  3. Use `filepath.Clean` to resolve the path, then verify the result is still within `absDir` with `strings.HasPrefix`.
  4. Use `http.Dir(absDir).Open(r.URL.Path)` instead of manual `os.Stat`+`ServeFile`.

---

## HIGH

### 2. `connect-src 'self' ws: wss:` Allows Data Exfiltration Over WebSocket to Any Host
- **File**: `internal/api/router.go:L49-L58`
- **Issue**: The CSP `connect-src` directive includes `ws:` and `wss:` as scheme sources. In CSP, a scheme source matches **any** origin using that scheme. This means `<script>` on the page (or an injected malicious script) can open WebSocket connections to arbitrary external servers (`ws://evil.com`, `wss://evil-c2.net`) and exfiltrate data. While `'self'` restricts `fetch`/`XHR` to same-origin, the `ws: wss:` entries bypass this restriction for WebSocket.

  The `'self'` source alone is insufficient for WebSocket when the page is served over HTTPS — browsers treat `ws://self` as a different origin from `https://self` and would block it. So `ws: wss:` is needed for legitimate WebSocket functionality, but the current CSP grants the WebSocket channel to *all* origins indiscriminately.

- **Impact**: If any XSS vulnerability exists (in group names, expense descriptions, member names, or a future feature), an attacker can use WebSocket to exfiltrate local storage tokens, session data, or group data to an external server — completely bypassing the `connect-src 'self'` restriction on HTTP fetch.
- **Fix**: Restrict WebSocket to the specific server origin using dynamic CSP construction:
  ```go
  wsHost := fmt.Sprintf("ws://%s", r.Host)
  wssHost := fmt.Sprintf("wss://%s", r.Host)
  // Set CSP with host-specific ws sources
  ```
  Alternatively, if the app only uses secure WebSocket (`wss://`), remove `ws:` from the CSP entirely. CSP Level 3 allows `ws://example.com` as a valid source expression in modern browsers (Chrome 96+, Firefox 94+, Safari 15.4+).

### 3. `absDir` from `filepath.Abs` Error Silently Ignored
- **File**: `internal/api/router.go:L108`
- **Issue**: The error from `filepath.Abs(staticDir)` is discarded with `_`. If `filepath.Abs` fails (e.g., due to path resolution issues), `absDir` will be the empty string `""`. When `absDir` is empty, `filepath.Join("", r.URL.Path)` resolves to `Clean("/" + r.URL.Path)` which is just `r.URL.Path` — an uncontrolled path relative to the current working directory.

  ```go
  absDir, _ := filepath.Abs(staticDir)
  ```

  While `os.Stat(staticDir)` must succeed before reaching this line, under edge conditions (filesystem race, network mount, or permission changes between `Stat` and `Abs`), `filepath.Abs` could behave unexpectedly. On some platforms with unusual path inputs, `Abs` can return an error even for paths that `Stat` reports as valid directories.

- **Impact**: If `absDir` is empty, the SPA fallback serves files from the root of the filesystem (`/`), effectively allowing reads of any file on the system without needing `..` traversal.
- **Fix**: Check the error from `filepath.Abs`:
  ```go
  absDir, err := filepath.Abs(staticDir)
  if err != nil {
      log.Printf("failed to resolve absolute path for %s: %v", staticDir, err)
      return
  }
  ```

---

## MEDIUM

### 4. `Promise.race` Timeout Does Not Cancel Underlying HTTP Requests
- **File**: `frontend/src/pages/GroupDetail.svelte:L243-L251`
- **Issue**: The 8-second safety timeout in `loadAll()` uses `Promise.race`, which only "wins" the race by resolving or rejecting with the timeout value — it does **not** abort or cancel the in-flight `fetch` requests. All four API calls (`group`, `expenses`, `payments`, `balances`) continue executing on the server and in the browser even after the timeout fires.

  ```javascript
  const data = await Promise.race([
      Promise.all([...api.get(...), ...]),
      new Promise((_, reject) => setTimeout(() => reject(new Error('Load timed out')), 8000)),
  ]);
  ```

  If the timeout fires, the four in-flight HTTP requests continue to completion, consume bandwidth, and their responses are discarded. Subsequent calls to `loadAll()` (e.g., triggered by WebSocket `setOnUpdate`) are gated by `_reloading`, so no new requests are made. The user is stuck in a "timed out" error state with no way to retry until `_reloading` clears.

  Furthermore, when the in-flight requests eventually complete, their responses can arrive and trigger state updates after the timeout has already fired. The `gen !== loadGen` guard catches this, but the responses are not abandoned until the four promises resolve (or reject), which could be much longer than 8 seconds if the server is slow.

- **Impact**: Under network congestion or server slowdown, the timeout fires, the user sees an error, but fetches continue in the background. The `_reloading` flag stays `true` until all four fetches resolve, so no retry can occur during that window. On slow networks, this can leave the page in a hung state for an extended period.
- **Fix**: Use `AbortController` to genuinely abort the fetch requests on timeout:
  ```javascript
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 8000);
  try {
      const data = await Promise.all([
          api.get(`/api/groups/${slug}`, token, { signal: controller.signal }),
          // ...
      ]);
  } finally {
      clearTimeout(timeoutId);
  }
  ```
  This also requires that `api.get` accepts and forwards an `AbortSignal`.

### 5. No Content-Length Set on JSON Fallback Error Path
- **File**: `internal/api/responses.go:L13-L16`
- **Issue**: When `json.Marshal(data)` fails in `respondJSON`, the error path calls `w.WriteHeader(500)` and `json.NewEncoder(w).Encode(...)` without setting a `Content-Length` header. The response uses chunked transfer encoding or connection close.

  ```go
  body, err := json.Marshal(data)
  if err != nil {
      w.WriteHeader(500)
      json.NewEncoder(w).Encode(map[string]string{"error": "json error"})
      return  // Content-Length never set
  }
  w.Header().Set("Content-Length", strconv.Itoa(len(body)))  // only set on success path
  ```

  While the error body (`{"error":"json error"}`) is short and the impact is minimal, the inconsistent header behavior means that a handler whose data fails to marshal sends a response without `Content-Length`. For HTTP/1.1 persistent connections, the client must rely on `Connection: close` or chunked encoding to detect the end of the body.

- **Impact**: Minor inconsistency. No direct security impact. For HTTP/1.1 keep-alive connections, the response may be interpreted incorrectly by aggressive proxies.
- **Fix**: Set `Content-Length` on the error path as well:
  ```go
  errBody := []byte(`{"error":"json error"}`)
  w.Header().Set("Content-Length", strconv.Itoa(len(errBody)))
  w.WriteHeader(500)
  w.Write(errBody)
  ```

### 6. `_reloading` Gate Blocks WebSocket-Triggered Refreshes During Initial Load
- **File**: `frontend/src/pages/GroupDetail.svelte:L229`
- **Issue**: The `_reloading` gate prevents concurrent `loadAll()` calls. However, during the initial page load, if a WebSocket update arrives (via `setOnUpdate(() => { loadAll(); })`) while loadAll is still in progress, the update-driven reload is silently dropped. The user may not see real-time updates until they manually refresh or the next periodic refresh cycle.

  This is by design (avoiding concurrent requests), but the combination with the 8-second timeout (Issue #4) means that if the initial load hangs, all real-time WebSocket updates are missed during that period, and the user sees stale data.

- **Impact**: Real-time collaboration breakage: a user makes a change, but other group members see stale data because WebSocket-triggered refreshes are blocked by a hanging `_reloading` flag.
- **Fix**: Queue a pending refresh request when `_reloading` is `true` and execute it after the current load completes. Alternatively, allow the update callback to bump a "pending refresh" counter and run `loadAll` once `_reloading` clears.

---

## LOW

### 7. `r.URL.Path` Normalization Differences Across Platforms
- **File**: `internal/api/router.go:L117`
- **Issue**: The path traversal (Issue #1) is the primary concern, but even in the absence of `..`, `r.URL.Path` handling varies. On Windows, `filepath.Join` would use `\` separators, potentially allowing different traversal patterns. The current `r.NotFound` handler also uses `os.Stat` which is OS-dependent. On case-insensitive filesystems, casing variations could bypass intended restrictions.
- **Impact**: Currently low given Linux deployment target, but portability concern.
- **Fix**: Use `http.FileServer` + `http.Dir` with `http.StripPrefix` for **all** static file serving, not just `/assets/*`. The `http.Dir` implementation has robust cross-platform path traversal protection.

### 8. Inline Favicon SVG Data URI Has XSS Surface
- **File**: `frontend/dist/index.html:L11`
- **Issue**: The favicon uses an inline SVG data URI. While CSP `img-src 'self' data:` allows it, constructing the SVG source from user-controlled data could enable SVG-based XSS. Currently the SVG is a static emoji, but if this pattern is extended to use user-provided group icons or avatars, it becomes an injection vector.
- **Impact**: Not exploitable in current code. Risk only for future feature additions.
- **Fix**: Document that SVG data URIs in `img-src` must never contain user-controlled content. Consider referencing a static SVG file instead.

---

## VERIFIED FIXES — No Remaining Issues

The following areas were reviewed and found to be correctly implemented:

### ✅ SPA Routing (Client-Side Routes)
The `r.NotFound` handler correctly falls back to `index.html` for paths that don't correspond to real files. SPA routes like `/groups/foo` and `/group/bar` (after API routes are exhausted) reach the NotFound handler, where `os.Stat` fails (no such file), and `index.html` is served. The client-side Svelte router then handles the path via hash or history API. This works correctly.

### ✅ Content-Length for Standard JSON Responses
`respondJSON` correctly marshals the full response body, sets `Content-Length` to the exact byte count, then calls `WriteHeader` and `Write`. The Content-Length header is set before `WriteHeader` (following Go's header protocol). All API responses that use `respondJSON`/`respondOK`/`respondError` benefit from this. The CORS OPTIONS handler (204 No Content) never reaches `respondJSON`, so no conflict exists.

### ✅ `_reloading` Race Condition Protection
The `_reloading` guard combined with `loadGen` generation counter correctly handles all race scenarios:
- Concurrent `loadAll` calls: Second call returns early via `_reloading == true`.
- Stale responses after re-mount: `gen !== loadGen` discard check in both `try` and `catch` blocks.
- Component unmount during load: Cleanup increments `loadGen`, and the in-flight `finally` block sees `gen !== loadGen` and skips state updates.
- Exception safety: All mutation paths are inside `try/finally`, and early returns explicitly reset `_reloading`.
- The cleanup function in `$effect` also sets `_reloading = false`, providing a safety net for edge cases.

### ✅ CSP for Service Worker Registration
`script-src 'self'` correctly allows `/registerSW.js` (a real file in the dist directory) and `/assets/index-*.js` (bundled scripts). The service worker itself (`/sw.js`) is controlled by `worker-src`, which falls back to `script-src 'self'` — also permitted.

### ✅ CSP for Inline SVG Favicon
`img-src 'self' data:` correctly allows the inline SVG favicon data URI in the HTML.

### ✅ CSP for WebSocket to Same Origin
The WebSocket connection uses `wss://` (or `ws://`) to the same host. For HTTPS pages, `wss://` is used. For HTTP, `ws://` is used. Both are covered by the `ws: wss:` entries in `connect-src`. While these allow connections to any host (Issue #2), the legitimate use case works correctly.

### ✅ CORS OPTIONS Handling
The CORS middleware correctly handles preflight `OPTIONS` requests with a 204 status and returns early, preventing any downstream processing. No Content-Length or Content-Type headers are conflictingly set for OPTIONS responses.

---

## Summary Table

| # | Area | Severity | Status |
|---|------|----------|--------|
| 1 | SPA file serving — path traversal | **CRITICAL** | **REGRESSION** — new vulnerability from the fix |
| 2 | CSP `connect-src ws: wss:` | **HIGH** | Existing, only partially mitigated |
| 3 | `filepath.Abs` error ignored | **HIGH** | Existing, not changed |
| 4 | Timeout doesn't cancel fetch | **MEDIUM** | Existing, not changed |
| 5 | Content-Length missing on error path | **MEDIUM** | Existing, minor |
| 6 | `_reloading` blocks WS updates | **MEDIUM** | Existing behavioral issue |
| 7 | Portability of path handling | **LOW** | Informational |
| 8 | Inline SVG favicon XSS surface | **LOW** | Informational |
| — | SPA client-side routing | ✅ | Verified correct |
| — | Content-Length JSON responses | ✅ | Verified correct |
| — | `_reloading`/`loadGen` race safety | ✅ | Verified correct |
| — | CSP service worker registration | ✅ | Verified correct |
| — | CSP inline SVG favicon | ✅ | Verified correct |
| — | CORS OPTIONS handler | ✅ | Verified correct |

**Most concerning finding**: The SPA fallback fix introduced a critical path traversal vulnerability. The use of `filepath.Join` with `r.URL.Path` is the pattern that `http.Dir` + `http.FileServer` was designed to replace. The `/assets/*` handler correctly uses `http.Dir` with traversal protection, but the `r.NotFound` handler bypasses this safety layer with manual file operations.
