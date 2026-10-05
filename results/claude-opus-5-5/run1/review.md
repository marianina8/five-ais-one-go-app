### Error handling
- **medium** — store.go:151-164, main.go:89-97 — `Visit` rewrites and fsyncs the whole data file on every redirect. If that write fails, for example because the disk is full or read-only, the redirect returns 500 even though the target URL is known. A broken disk therefore takes down every short link, not just the statistics. In a scratch test I deleted the data directory after creating a link, and `GET /abc` returned 500 `{"error":"internal error"}` [verified]. Fix: always serve the redirect. Log a failed visit-count save, or keep counts in memory and flush them periodically.
- **medium** — main.go:70-77, 89-97, 166-174 — Every 500 path returns "internal error" and the underlying error is never logged. The disk failure above leaves no trace anywhere, so an operator can't diagnose it. Fix: call `log.Printf("...: %v", err)` before each 500 response. Don't put the error text in the response body.
- **low** — store.go:98-101 — The directory `Open` and `Sync` errors are ignored. The rename has already happened, so the data is probably fine, but the "atomic and durable" claim in the comment isn't guaranteed. Fix: return or log the error from the directory sync.
- **low** — main.go:28 — `writeJSON` ignores the `Encode` error. This is mostly harmless, but a failed write to the client goes unnoticed. Fix: log the error.
- **low** — store.go:50-57 — `OpenStore` silently drops entries with duplicate codes, and the next save then permanently removes them from the file. Fix: return an error, or at least log the dropped entries.

### Secrets and sensitive data
- **low** — main.go:142 — `"invalid JSON: "+err.Error()` returns raw decoder error text to the client. It contains no secrets, but it exposes internal details. Fix: return a fixed message.
- No secrets are logged. The admin token comes from the environment and is compared in constant time (main.go:110).

### Input validation
- **high** — main.go:43-46, 128-176 — `POST /api/links` needs no authentication, while list and delete do. Anyone who can reach the server can create unlimited links. That allows phishing redirects hosted on your domain, and it lets an attacker grow the data file without limit. Every create rewrites and fsyncs the full file, so the cost per create rises as the file grows. A scratch test confirmed that an unauthenticated create returns 201 [verified]. Fix: require the token for create, or add rate limiting and a cap on the number of links. If open creation is intended, document it and add those limits.
- **low** — store.go:44-57 — Entries loaded from the data file skip the URL and code validation that `create` applies. A hand-edited file could contain a code with `/` that can't be reached, or a non-http URL that gets sent in a `Location` header. Fix: run `validURL` and `aliasRe` on load, or skip bad entries and log them.
- **low** — main.go:85-98 — `HEAD` requests, such as link-preview bots and uptime checks, increment the visit counter and trigger a full file write. Fix: don't count HEAD requests.

### External services
- **medium** — main.go:194 — `http.ListenAndServe` uses the default server, which has no `ReadHeaderTimeout`, `ReadTimeout` or `IdleTimeout`. Slow clients can hold connections open indefinitely. There is also no graceful shutdown. Fix: build an `http.Server` with timeouts, and call `Shutdown` on SIGTERM.

### Resources and concurrency
- **medium** — store.go:151-164, 62-103 — One global mutex is held during the whole file rewrite, including fsync, for every redirect and every create. Redirects are therefore serialized behind disk I/O, and the cost grows with the number of links. This is correct but won't scale. Fix: update visit counts in memory and persist them in the background (batched), or use an append-only log or a database.
- **low** — store.go:78-82 — `os.Remove(tmp)` runs only when `ok` is false, so a failed directory sync can't leave a stray file. That is fine. However, a crash between `CreateTemp` and rename leaves `.shortener-*.tmp` files behind, and nothing cleans them up. Fix: remove stale temp files on startup.

### Tests
- **medium** — main_test.go — No test covers a failing save. The rollback code in `Create`, `Visit` and `Delete` (store.go:143-147, 159-162, 205-209) is never exercised. Fix: make the data directory read-only or remove it, and assert that the in-memory state is unchanged and the right status is returned.
- **low** — main_test.go — Missing tests: a bad or malformed `Authorization` header, `HEAD` handling, `DELETE` on a nested path (404) or with a wrong method (405), `OpenStore` on a corrupt or empty file, and random-code collision handling.
- **low** — main_test.go:51, 85 — `json.Unmarshal` errors are ignored. A malformed response would show up as a confusing later assertion failure instead of a clear one.
- **low** — main_test.go:116-117 — The goroutines in `TestConcurrent` ignore the response codes. The final count check does catch lost writes. The test would be more precise if it asserted 201 and 302 as well.

### Clarity
- **low** — store.go:186-211 — `Delete` copies the order slice, then splices the original in place, then restores from the copy on failure. This works but is hard to follow. `idx >= 0` is always true when the code exists in `links`. Fix: build the new order with `slices.Delete` (or a new slice), save, and only then commit the change. Better still, make a shared "mutate, save, roll back on error" helper that `Create`, `Visit` and `Delete` all use.
- **low** — main.go:145-153 — `dec.Token(); err != io.EOF` is a clever way to detect trailing data. If `Token` returns a valid token with a nil error, the code reports "trailing data", which is correct but not obvious. Fix: add a comment saying "after the first JSON value, the only acceptable result is EOF."
- **low** — main.go:81 and 161, store.go:134 — The reserved name `"api"` is checked in three places. Fix: define one `isReserved(code)` helper.
