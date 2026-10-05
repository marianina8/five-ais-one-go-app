### Error handling
- **medium** — server.go:166-193 (with server.go:24-26, 113, 123, 153) — The 404/405 interceptor re-wraps JSON that the handlers already wrote. A missing code, `/api`, or a delete of a nonexistent code returns `{"error":"{\"error\":\"not found\"}"}`, which is JSON nested inside a JSON string. Only mux-generated errors such as `/a/b` come out correctly. Fix: in `Write`, only rewrite the body when the handler did not already set `Content-Type: application/json`. A cleaner way is to remember in `WriteHeader` that the interceptor changed the header, and rewrite only in that case. [verified]
- **medium** — server.go:57 — Detects an oversized body by comparing the error text. This breaks if the message changes and misses wrapped errors. Fix: `var mbe *http.MaxBytesError; if errors.As(err, &mbe)`.
- **medium** — store.go:107-122 and server.go:117-121 — Every redirect rewrites and fsyncs the whole data file while holding the global lock. If the disk write fails, the visitor gets a 500 instead of their redirect. Fix: count visits in memory and flush periodically or in a background goroutine, or at least redirect even when saving fails and log the error.
- **low** — server.go:38 — The `rand.Read` error is ignored. `r[0]%62` is also slightly biased, since 256 is not a multiple of 62. Fix: read all 7 bytes at once, check the error, and use `rand.Int` or reject values ≥ 248.
- **low** — server.go:21, 188-189 — Errors from `Encode` and `Write` are dropped, and `json.Marshal` errors are discarded with `_`. This is harmless for a client that disconnects, but unlogged. Fix: log them.
- **low** — server.go:54-63 — `Decode` reads one JSON value and ignores anything after it. `{"url":"http://a.com"} garbage` returns 201. Fix: after `Decode`, check that `dec.More()` is false, or that a second `Decode` returns `io.EOF`. [verified]
- **low** — store.go:42-49 — A data file containing `[null]` makes `NewStore` panic with a nil dereference at line 47 instead of returning an error. Duplicate codes in the file silently overwrite each other in `byCode`. Fix: validate each entry on load and return an error. [verified]
- **low** — store.go:59-78 — If the write, sync or rename fails, the `.tmp` file is left behind. The directory is not fsynced after the rename, so the "crash safe" comment holds only partly. Fix: remove the tmp file on error. Optionally fsync the directory.

### Secrets and sensitive data
- **medium** — server.go:28-31 — The bearer token is compared with `==`, which is not constant-time and allows timing attacks. Fix: use `subtle.ConstantTimeCompare` on the two values (hash both first, or compare lengths separately).
- **low** — store.go:61 — The data file is created with mode 0644. It holds only links, so the exposure is small, but 0600 is safer. Fix: use 0600.

### Input validation
- **medium** — server.go:66-70 — A URL is accepted if it parses with an http or https scheme and a host. Links containing credentials (`http://user:pass@host`) are accepted and stored. The 2048-character check runs after parsing, so a URL up to 1 MB is parsed first. Fix: check the length first, and reject `u.User != nil`.
- **low** — server.go:76, 87 — Only `api` is reserved. This is fine for now because every other route has a deeper path or a different method. A future route like `/healthz` would conflict with aliases. Fix: keep a reserved-words set shared by both checks.
- **low** — server.go:84-105 — If `generateCode` keeps colliding, the loop retries forever. Fix: cap the retries at around 10 and return 500.

### External services
- **medium** — main.go:39 — `http.ListenAndServe` uses a server with no `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` or `IdleTimeout`. A public service is open to slowloris attacks. There is also no graceful shutdown, so an in-flight `save` can be cut off by SIGTERM. Fix: build an `http.Server` with timeouts and call `Shutdown` on a signal.
- **low** — store.go:107-122 — Clients pay for disk I/O on every GET, and there is no rate limiting on `POST /api/links`, which anyone can call without authentication. Anyone can fill the data file. Fix: rate-limit or cap the number of links.

### Resources and concurrency
- **medium** — server.go:107, 137 and store.go:104, 167-172 — `Create` and `List` return live `*Link` pointers, and the handler JSON-encodes them after the lock is released. Meanwhile `Visit` does `l.Visits++` under the lock. This is a data race on `Visits`, so a list response can race a redirect. Fix: return `Link` values (copies) from `Create`, `Visit` and `List`. I did not run this under `-race`, so it is not verified.
- **low** — store.go:142 — `Delete` shifts `s.links` in place, and the revert at line 153 rebuilds the slice. This works, but it is easy to get wrong and was never tested. Fix: build a new slice, or write the file first and then change memory.

### Tests
- **medium** — server_test.go:119-136 — The tests never check the body of a 404 from a handler (missing link, deleting a nonexistent code), so the double-encoding bug passes unnoticed. Fix: assert that the body decodes to `{"error":"not found"}`.
- **medium** — server_test.go:139-174 — The concurrency test is not run with `-race`, and it never overlaps `List` with `Visit`, the case where the race is. Fix: run CI with `go test -race` and add a test that calls List while Visit runs.
- **low** — server_test.go:57, 90, 133, large_test.go — Decode errors are ignored, and several response bodies are not closed. Setup errors are discarded with `_`. Fix: check them with `t.Fatal`.
- **low** — large_test.go:13 — The test uses `&Store{}` with a nil map. It passes only because the request is rejected before the store is used, which hides the dependency. Fix: use `NewStore` on a temp file.
- **low** — server_test.go:31-37 — The test repeats the route wiring from main.go, so the two can drift apart. Fix: extract a `(s *Server) routes() http.Handler` and use it in both.
- **low** — mux_test.go, mux_intercept_test.go — Both files contain only `package main`. Remove them or add tests.
- **low** — (no test) — Missing coverage for: invalid or duplicate alias (409), invalid URL, 401 without a token, `Delete` or `Create` revert on save failure, and loading a corrupt data file.

### Clarity
- **low** — server.go:73-105 — The create loop combines an `isAlias` flag, a `continue` for the `api` code and a retry on duplicate, so the flow is hard to follow. In plain terms: use the alias if given, otherwise generate random codes until one is free. Suggested: `if req.Alias != "" { link, err = s.store.Create(req.Alias, req.URL) } else { link, err = s.createRandom(req.URL) }`, then handle `ErrDuplicateCode` and other errors once. Put the retry loop with a cap in `createRandom`.
- **low** — server.go:160-200 — The interceptor decides what to do from `w.status` plus the current `Content-Type` header, and it rewrites bodies by matching on their text. Reading it, you can't tell which responses it changes. Suggested: set a `rewrite` boolean in `WriteHeader` only when the status is 404 or 405 and the handler has not set a content type. In `Write`, if `rewrite` is set, emit the JSON error. Otherwise pass the bytes through.
- **low** — store.go:124-158 — `Delete` first removes the item, then fetches `oldLink` afterwards, and then has a revert path with a nested `append(append(...))`. Suggested: look up `oldLink` at the top, find the index, save, and only then mutate memory (or write the new slice first and swap it in on success). This removes the revert code.
