### Error handling
- **medium** — store.go:163-166, server.go:117-125 — A redirect is also a disk write: `Visit` saves the whole file to count the visit. If the save fails (disk full, directory gone, bad permissions), the visitor gets a 500 and no redirect, even though the link is valid and the URL is in memory. A broken disk therefore takes down every short link. I confirmed this with a scratch test: after pointing the store at an unwritable path, `GET /hot` returned 500 `{"error":"internal error"}` instead of 302 [verified]. Fix: always redirect. Log a failed visit-count save and carry on, or update the counter in memory and flush it in the background.
- **medium** — server.go:87, 123, 195 — Every 500 path discards the real error, and nothing is logged anywhere (`log` is only used at startup in main.go). When a save fails, the operator sees only "internal error" and has nothing to debug with. Fix: call `log.Printf("create: %v", err)` (or similar) before each generic 500. Don't log the request body or the Authorization header.
- **low** — server.go:31 — `json.NewEncoder(w).Encode(v)` ignores its error. Fix: log it, or at least comment that the write failure is intentionally ignored.
- **low** — store.go:100-103 — The directory fsync error is ignored, so the "persists atomically" claim in the comment may not hold. Fix: return or log the error.
- **low** — store.go:52-59 — `OpenStore` silently skips duplicate codes and accepts records with an empty or invalid code (for example "api" or one with a slash). Those links can never be reached or deleted. Fix: validate each record on load, and fail or log on duplicates.

### Secrets and sensitive data
- **low** — server.go:137-150 — `validURL` accepts URLs with embedded credentials, such as `https://user:pass@host/`. These are stored in plaintext in data.json and returned by `GET /api/links`. Fix: reject `u.User != nil`.
- **low** — server.go:43-51 — The token check is constant-time, but `ConstantTimeCompare` returns early when the lengths differ, so the token length leaks. This is minor. Fix: compare SHA-256 hashes of both values. Also note that the token travels in cleartext unless the service sits behind TLS.

### Input validation
- **medium** — server.go:152-199, main.go:27-31 — `POST /api/links` needs no authentication and has no rate limit or cap on the number of links. Anyone can create links without limit. Each create rewrites the full file under a global lock, so this also becomes a disk-growth and CPU problem. Fix: require the token (or document that open creation is intended), and add a rate limit or a maximum link count.
- **low** — store.go:123-142 — `Store.Create` doesn't validate the alias itself. It relies on the HTTP handler to reject "api" and malformed aliases. A caller that bypasses the handler can create bad codes. Fix: move the alias check into the store.
- **low** — main.go:13-16 — The `-addr` and `-data` flags aren't checked. The data directory isn't verified as writable at startup, so a bad path only fails on the first write. Fix: do a trial `saveLocked` at startup.

### External services
- **medium** — main.go:27-31 — `http.Server` sets only `ReadHeaderTimeout`. There is no `ReadTimeout`, `WriteTimeout` or `IdleTimeout`, so slow-body clients can hold connections open indefinitely. Combined with the open `POST`, this is easy to abuse. Fix: set the missing timeouts.
- **low** — main.go:33 — There is no graceful shutdown. A SIGTERM during a save could leave a stray `.shortener-*.tmp` file. Fix: use `signal.NotifyContext` and `srv.Shutdown`.

### Resources and concurrency
- **medium** — store.go:72-105, 155-168 — One global mutex is held across marshal, write, fsync, rename and directory sync for every operation, including every redirect. Cost per request grows with the total number of links, and all redirects are serialised behind disk I/O. Fix: keep reads and redirects off the disk path. Use an `RWMutex` for `Get` and `List`, and persist visit counts asynchronously or in batches.
- **low** — store.go:83-98 — The temp-file cleanup is spread over several branches and is easy to get wrong when editing. Fix: use one `defer` that removes the temp file unless the rename succeeded.

### Tests
- **medium** — server_test.go — No test covers a failing save, so the 500-on-redirect behaviour above went unnoticed. Rollback in `Create`, `Visit` and `Delete` is also untested. Fix: add a test with an unwritable data path, like my scratch test.
- **low** — server_test.go:82-93 — The `DELETE` calls in `TestFlow` are only made with the token. Nothing checks that a `DELETE` without a token (or with a wrong one) returns 401, or that a wrong-token `GET /api/links` is rejected. Fix: add those cases.
- **low** — server_test.go:119-150 — `TestConcurrent` ignores HTTP errors and status codes, so failed requests only show up indirectly in the final counts. It is also unclear whether it was run with `-race`. Fix: assert on status codes and run with `go test -race`.
- **low** — server_test.go — There is no test for `HEAD` (which skips the visit count), for trailing JSON data, for corrupt or empty data files in `OpenStore`, or for 405 on a known short code.

### Clarity
- **low** — server.go:53-130 — `ServeHTTP` is one long switch with nested branches for routing, auth, method checks and redirect logic. The redirect section (lines 108-128) is the hardest part. For `HEAD` it calls `Get` and builds an `ErrNotFound` by hand, and for `GET` it calls `Visit`. It does not count visits for `HEAD`, and that is not stated anywhere. Fix: split it into `handleList`, `handleDelete` and `handleRedirect`. In `handleRedirect`, write it plainly: for `HEAD` look up the link, for `GET` count the visit, and never fail the redirect because of the count.
- **low** — server.go:158-168 — The "nothing but whitespace follows" check decodes into a `RawMessage` and juggles `io.EOF` and error values. It works but is dense. Fix: pull it into a small `decodeStrict(r, &req)` helper that returns a clear error.
- **low** — store.go:189-214 — `Delete` copies the whole order slice for rollback and edits `s.order` in place. Fix: save first and remove from memory only after the save succeeds, so no rollback is needed. A simpler approach is to build the new order slice, save, then swap it in.
