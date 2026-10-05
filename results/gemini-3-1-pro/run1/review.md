### Error handling
- **medium** — store.go:131-135, server.go:97-106 — Every redirect first rewrites the whole data file to bump the visit counter. If that write fails (disk full, read-only directory), `GetAndVisit` returns an error and the handler sends a 500. A working short link then stops redirecting because of a statistics failure. I made the data directory read-only and got a 500 on `GET /abc` [verified]. Fix: serve the redirect anyway and log the save error. Better, keep counts in memory and flush them periodically.
- **medium** — store.go:41-47 — `NewStore` doesn't validate the loaded file. A file containing `[null]` makes it panic with a nil pointer dereference at line 46 instead of returning an error [verified]. Duplicate codes in the file silently overwrite each other in `linkMap` while both stay in `links`. Fix: reject nil entries and empty or duplicate codes, and return an error.
- **medium** — store.go:199 — `rand.Int` returns an error that is discarded. If it ever fails, `n` is nil and `n.Int64()` panics. Fix: return the error from `generateRandomCode` (or panic deliberately with a clear message). Also cap the retry loop at lines 94-99 so it can't spin forever.
- **low** — server.go:81, 99, 120 — Errors are matched by comparing `err.Error()` to the strings "conflict" and "not found". A reworded message silently turns a 409 or 404 into a 500. Fix: define `var ErrConflict = errors.New(...)` and `var ErrNotFound = errors.New(...)`, return them from the store, and test with `errors.Is`.
- **low** — server.go:22, 92, 113 — `json.Encoder.Encode` errors are ignored. This is acceptable for a client disconnect, but at least log them.
- **low** — store.go:66-80 — The temp file is renamed over the data file without fsyncing the directory, so the rename can be lost after a power failure. Also, `os.Remove(tmpName)` is deferred, so it runs after a successful rename and always fails harmlessly. Fix: fsync the directory if durability matters.

### Secrets and sensitive data
- **medium** — server.go:28-29 — The admin token is compared with `!=`, which is not constant-time and so allows timing attacks. Fix: use `subtle.ConstantTimeCompare` on the supplied token and the expected token.
- **low** — main.go:16-20 — The token comes from the environment, which is reasonable. There is no TLS, and the bearer token travels in cleartext unless a reverse proxy terminates TLS. Document this requirement.

### Input validation
- **high** — server.go:176 — `POST /api/links` has no auth, while list and delete do. I sent a create request with no token and got 201 [verified]. Anyone on the network can add redirects to any URL, which makes this an open redirector for phishing. They can also grow the data file without limit and reserve any alias. Every create also rewrites the whole file. If this is intentional, add rate limiting and a cap on the number of links. Otherwise wrap it in `requireAuth`.
- **low** — server.go:45-55 — Only the first JSON value in the body is decoded, so trailing data such as `{"url":...}garbage` is accepted. Fix: after `Decode`, check that `dec.More()` is false, or that a second `Decode` returns `io.EOF`.
- **low** — server.go:62-66 — The URL check accepts any http or https host, including `localhost`, internal IPs, and URLs with embedded credentials (`user:pass@host`). This matters mostly for the open create endpoint above. Fix: decide on a policy and reject those cases.
- **low** — server.go:68-77 — Alias reservation only blocks `api`. That is enough for the current routes, but it is a hardcoded special case. Add new reserved names there when routes are added.

### External services
- **medium** — main.go:30 — `http.ListenAndServe` uses a server with no `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` or `IdleTimeout`. Slow clients can hold connections open indefinitely (slowloris). There is also no graceful shutdown on SIGINT or SIGTERM. Fix: build an `http.Server` with timeouts and call `Shutdown` on a signal.
- **medium** — store.go:54-81, 122-138 — Each request that changes state (create, delete, and every redirect) marshals the entire list, writes a temp file, fsyncs it, and renames it while holding the global mutex. Throughput is limited by disk fsync latency and gets worse as the data grows. Public redirect traffic therefore serializes on disk I/O. Fix: use read-only lookups for redirects and batch or asynchronously flush the visit counts.

### Resources and concurrency
- **low** — store.go:140-151 — `List` copies correctly under the lock, so there is no race. `GetAndVisit` returns the internal `*Link` pointer, and the handler reads `link.URL` after the lock is released. `URL` is never mutated, so this is safe today, but it is fragile. Fix: return a copy.
- **low** — server_test.go:24-85 — Response bodies are never closed in the tests, which leaks connections within the test run. Close them with `defer res.Body.Close()`.
- **low** — server.go:130-170 — `rwWrapper` hides the optional `http.Flusher` and `http.Hijacker` interfaces of the underlying writer. Nothing uses them now, but it can bite later.

### Tests
- **medium** — large_test.go:10-24, server_test.go — No tests cover the failure paths. Missing cases: save failure with rollback in `Create`, `Delete` and `GetAndVisit`; corrupt or `null` data files; `DELETE` with a bad token; concurrent access (run with `-race`); and the 7-character random code collision loop. The rollback code at store.go:112-116, 183-189 has no coverage.
- **low** — time_test.go:10-16 — `TestRFC3339` can't meaningfully fail. `time.Now().UTC()` always marshals with `Z`, and the assertion also accepts any `+`. It doesn't test `Create`'s millisecond truncation either. Fix: create through the store and parse the output with `time.Parse(time.RFC3339Nano, ...)`.
- **low** — server_test.go:32, 39, 46, 62, 72, 123-126 — Errors from `Decode`, `http.Post`, `NewStore` and `Create` are discarded. A failed request leaves `res` nil and the test panics with a nil dereference instead of a clear failure message.
- **low** — mux_test.go, test_mux.go, test_time.go — These files are empty stubs (the `test_*.go` ones are `//go:build ignore`). They are dead files that suggest tests which don't exist. Delete them.

### Clarity
- **low** — server.go:130-170 — `rwWrapper` rewrites the response body for any 404 or 405. It discards whatever the handler wrote, and returns `len(b)` to pretend the write succeeded. The goal is JSON errors for the mux's built-in plain-text 404 and 405 responses. Because the wrapper replaces every 404 body, the handlers' own `writeJSONError` 404s are overwritten too. It works, but it is surprising. Plainer approach: register a catch-all `/` handler for 404, and give each route a method-not-allowed fallback that calls `writeJSONError`.
- **low** — store.go:161-171 — `Delete` already checks `linkMap`, then scans the slice again and has an "impossible" error branch. Keeping a slice and a map in sync costs extra code. Fix: store links in one structure, or use `slices.DeleteFunc`. The delete rollback at lines 185-188 (append nil, copy, assign) is also hard to read. Simpler: build the new slice first, save it, and only assign `s.links` on success.
- **low** — store.go:105 — `time.Now().Truncate(time.Millisecond).UTC()` works but the order is odd. Use `time.Now().UTC().Truncate(time.Millisecond)`.
