### Error handling
- **medium** — store.go:150-163 — Every redirect rewrites and fsyncs the whole data file, and a failed write returns a 500 instead of the redirect. A full disk or a read-only data directory therefore breaks all existing short links, just because a visit counter couldn't be saved. I ran this by deleting the data directory and requesting `/abc`. The response was `500 {"error":"internal error"}` and the log showed `open …/d.json.tmp…: no such file or directory` [verified]. Fix: treat the counter as best-effort. Log the save error and still return the URL, or keep counts in memory and flush them periodically in the background.
- **medium** — store.go:47-56 — On load, entries that are nil, have an empty code, or have a duplicate code are silently skipped. The next `save()` then rewrites the file without them, so data that was in the file is deleted without any message. I loaded a file with a duplicate and an empty-code entry, then created a link. The file no longer contained the skipped entries [verified]. Fix: return an error from `OpenStore` (or at least log each skipped entry), and don't overwrite the file until an operator has dealt with it.
- **low** — store.go:92-95 — The error from the directory `Sync` and `Open` is ignored, so the claim in the comment (the rename is durable) may not hold. Fix: return or log the error.
- **low** — main.go:28 — The error from `Encode` is ignored. This is harmless for most cases, but a failed write to the client leaves no trace. Fix: log it.

### Secrets and sensitive data
- **low** — main.go:179-183 — The token comes from an environment variable and is never logged, which is fine. There is no minimum length check, so `ADMIN_TOKEN=x` is accepted and a trivially guessable admin token is possible. Fix: require a minimum length, such as 16 characters or more.

### Input validation
- **medium** — main.go:57-58 — `POST /api/links` needs no authentication. Anyone who can reach the server can create unlimited links to any http(s) URL, such as phishing targets, and each one grows the data file. Since every write rewrites the whole file, this also makes the server slower for everyone. Fix: require auth for creation, or add rate limiting and a cap on the number of links, unless a public endpoint is intended (if so, document it).
- **low** — store.go:43-56 — URLs and visit counts loaded from the file are not validated. A hand-edited or corrupt file can put an arbitrary string into the `Location` header. Fix: run `validURL` on load and reject negative visits.
- **low** — store.go:33-36 — `OpenStore` accepts a path whose directory doesn't exist, so the first write fails at runtime, not at startup [verified]. Fix: check at startup that the directory is writable.

### External services
- **high** — main.go:189 — `http.Server` has no `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` or `IdleTimeout`. Slow clients (slowloris) can hold connections open indefinitely. Fix: set these fields, for example `ReadHeaderTimeout: 5s`, `ReadTimeout: 10s`, `WriteTimeout: 10s` and `IdleTimeout: 60s`.
- **low** — main.go:189-193 — There is no graceful shutdown. A SIGTERM during a save can kill the process mid-request. The temp-file rename keeps the data file intact, but it leaves stray `.tmp*` files and drops in-flight requests. Fix: handle signals and call `srv.Shutdown`.

### Resources and concurrency
- **medium** — store.go:118-163 — The global mutex is held while doing disk I/O: marshal, write, fsync and rename. Every redirect, which is the hot path, queues behind a full rewrite of the file. Cost grows with the number of links, and `List` blocks as well. Fix: don't persist on visits (see above), or move writes to a single background writer.
- **low** — store.go:73-78, 91-95 — Temp files are cleaned up on error, but a crash between `CreateTemp` and `Rename` leaves `d.json.tmp*` files behind, and nothing removes them at startup. Fix: clean up stale temp files in `OpenStore`.

### Tests
- **medium** — main_test.go — None of the persistence failure paths are tested. This covers the rollback in `Create` (store.go:141-145), `Visit` (158-161) and `Delete` (182-186), and the 500 responses. Fix: make the data directory unwritable (or delete it) and assert the in-memory state is unchanged and the status is 500.
- **low** — main_test.go — No test for loading a corrupt file, a file with duplicate or empty codes, or `randomCode` output (length and alphabet). Fix: add table tests for these.
- **low** — main_test.go:45, 70 — The `json.Unmarshal` errors are ignored, so a broken response body would only show up as a later, confusing assertion failure. Fix: check the errors.
- **low** — main_test.go:33-93 — `TestFlow` is one long chain of assertions with bare `t.Fatal(w.Code)` messages. If it fails, it doesn't say which step broke. Fix: split it into subtests and use messages like `t.Fatalf("create duplicate: got %d, want 409", w.Code)`.
- **low** — main_test.go:120 — `var _ = http.StatusOK` exists only to keep an unused import alive. Fix: remove it and the `net/http` import.

### Clarity
- **low** — store.go:172-186 — `Delete` does two odd things. It copies the whole slice into `old` for rollback, and it uses `append(s.links[:pos:pos], s.links[pos+1:]...)`. The three-index slice forces `append` to allocate a new array, which is why `old` stays valid. That reasoning is not obvious. Fix: keep the original slice as `old` and build the new one explicitly: `next := make([]*Link, 0, len(old)-1); next = append(next, old[:pos]...); next = append(next, old[pos+1:]...)`. Then only swap it in after a successful save.
- **low** — main.go:133-150 — The "request body too large" handling is written twice. The trailing-data check `dec.Token(); err != io.EOF` is also hard to read: it means "anything after the first JSON value is an error". Fix: write a small `decodeBody(w, r, &req) bool` helper with a comment explaining the trailing-data check.
- **low** — main.go:38-39 — The Bearer parsing is dense: a length check, a case-insensitive prefix compare and a constant-time compare, all in one condition. Fix: use `strings.CutPrefix`-style parsing in a separate `bearerToken(r)` function and call `subtle.ConstantTimeCompare` on the result.
- **low** — store.go:101-116 — `randomCode` rejection sampling is correct but cryptic. `max` is the largest multiple of 62 that fits in a byte, and values at or above it are discarded to avoid modulo bias. Fix: add a one-line comment saying that. `max` also shadows the Go 1.21 builtin, so rename it to `limit`.
