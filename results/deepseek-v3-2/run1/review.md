### Error handling
- **high** — store.go:230-243 (also 191-205) — `Delete` removes the link from memory and then saves. If the save fails, it returns an error but does not put the link back. The API reports a 500, yet the link is gone from memory and still on disk. After a failed delete, `Get` returned "not found" and the file still contained the link. It comes back after a restart. `Create` (store.go:167-172) does roll back, so the three write paths behave inconsistently. `IncrementVisits` also keeps the changed count in memory when the save fails. Fix: copy the old value, mutate, save, and restore the old value if the save fails. Or build the new map, save it, and swap it in only on success. [verified]
- **medium** — main.go:120-129, 162, 179 — Errors are matched by their text (`err.Error() == "not found"`, `"alias taken"`, and so on). Changing a message silently turns a 404 or 409 into a 500. Also, `Create` wraps the save error but returns the bare "failed to generate unique code" (store.go:144), which falls into the default 500 branch. Fix: define sentinel errors (`var ErrNotFound = errors.New("not found")` and similar) and use `errors.Is`. The tests (`strings.Contains(err.Error(), ...)`) can use them too.
- **medium** — main.go:112-115 — Every decode error returns 400 "Invalid JSON". A body over 1 MiB gets 400, but the README (line 110) promises 413. Fix: check `errors.As(err, new(*http.MaxBytesError))` and return 413. Also reject trailing data after the JSON object. [verified: a 2 MiB body returned 400]
- **medium** — store.go:81-88, 107-114 — The temp file is never fsynced and the directory is not synced. The write is not safe against power loss, which the README claims ("even during crashes"). If the rename fails, the stale `.tmp` file stays behind. Fix: use `os.CreateTemp` in the same directory, then `f.Sync()`, `f.Close()` and `os.Rename`. Remove the temp file on error.
- **low** — main.go:84 — The error from `json.NewEncoder(w).Encode` is ignored. This is minor, but a failed write goes unlogged.
- **low** — store.go:52-60 — `load` does not check the stored data. Duplicate codes are silently merged, and entries with an empty code or a bad URL are accepted. Fix: validate on load, or at least reject duplicates.

### Secrets and sensitive data
- **medium** — main.go:99 — The admin token is compared with `==`, which is not constant-time. Also the `Bearer` prefix is case-sensitive, so `bearer tok` gets 401 (verified). Fix: use `subtle.ConstantTimeCompare([]byte(token), []byte(h.adminToken))` and `strings.EqualFold` for the scheme. Weak tokens are accepted, since any non-empty value passes.
- **low** — store.go:81, 107 — The data file is written with mode 0644, so any local user can read it. Use 0600 if the links should stay private.
- **low** — store.go:246-265 — URLs containing credentials (`http://user:pw@example.com/`) are accepted, stored in the data file, returned by the admin list endpoint, and sent in the redirect (verified). Fix: reject `parsed.User != nil`.
- **low** — store.go:7, 289 — Random codes come from `math/rand`, so they can be predicted. If unlisted links should stay unguessable, use `crypto/rand`.

### Input validation
- **medium** — store.go:246-265 — `validateURL` only checks the scheme and that the host is non-empty. Fix: also reject URLs with user info (see above).
- **low** — main.go:107-115 — Unknown JSON fields and trailing garbage are accepted. Use `DisallowUnknownFields` if you want strict input.
- **low** — main.go:73 — The `/{code}` route accepts any path, including `/a/b/c` and encoded characters. It does a map lookup, so this is harmless, but it can return a 404 for paths that should be rejected. `HEAD` requests also get 404 (verified).
- **low** — main.go:12-31 — `-addr` and `-data` are not validated. A missing data directory only fails on the first write, not at startup. Fix: check the directory is writable at startup.

### External services
- **high** — main.go:30 — `http.ListenAndServe` is used with no `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` or `IdleTimeout`, so slow clients can hold connections open (Slowloris). Fix: build an `http.Server` with those timeouts and shut it down gracefully on SIGTERM.
- **medium** — main.go:188-192, store.go:191-205 — Every redirect starts a new goroutine. Each goroutine takes the global write lock and rewrites and renames the whole JSON file. `POST /api/links` is open to anyone, so a flood of redirects or creates causes unbounded goroutines, heavy disk I/O, and lock contention that also blocks readers. Fix: keep the visit count in memory and flush it periodically or on shutdown. Or use a single worker goroutine fed by a buffered channel. Consider rate-limiting create.
- **low** — main.go:105-117 — Link creation needs no authentication. If the spec requires this, fine, but there is no limit on the number of links either. The data file grows without bound.

### Resources and concurrency
- **medium** — main.go:188-192 — The goroutine is fire-and-forget. Nothing waits for it at shutdown, so a visit can be lost. In tests it can also run after the temp directory is removed. Fix: use a `WaitGroup` or a worker with a shutdown signal.
- **medium** — store.go:230-243 — All disk I/O happens while holding the write lock. The lock blocks redirects (`Get` uses `RLock`) for the duration of every file write.
- **low** — store.go:66-91 — `save()` is never called and duplicates `saveLocked()`. It also releases the lock before writing, which would race with other writers on the shared `.tmp` file. Remove it.
- **low** — README.md:118 — It says visit counters are "incremented atomically". They are only mutex-protected, and a failed save leaves memory and disk out of sync.

### Tests
- **medium** — store_test.go, handler_test.go — Nothing tests a failed save. That is exactly where the Delete and IncrementVisits bugs hide. Add a test that makes the data path unwritable (for example a directory at `data.json.tmp`) and checks the state afterwards.
- **medium** — handler_test.go:303-324 — `TestHandler_LargeRequestBody` accepts either 400 or 413. The body is all zero bytes, so it fails as invalid JSON before the size limit matters, and the test would pass without `MaxBytesReader`. Use valid JSON larger than 1 MiB and require 413.
- **medium** — store_test.go:21 — The "too long" URL is built from null bytes. It would be rejected as invalid even without the length check, so the length check is not really tested. Use `"http://example.com/" + strings.Repeat("a", 2100)`.
- **low** — handler_test.go:326-338 — `TestMain_EnvironmentValidation` asserts nothing and only logs. It cannot fail. Delete it, or move the check into a testable function.
- **low** — handler_test.go:164-222, store_test.go:178-222 — The list-order test sleeps 1 ms to make timestamps differ and only checks "not decreasing". Many other cases are missing, such as wrong-token and malformed `Authorization` headers, and the redirect incrementing visits (the async increment is never checked). The tests also never run under `-race`.
- **low** — store_test.go:325-339 — `TestGenerateRandomCode` can fail by chance, though only rarely. It also does not check the character set.

### Clarity
- **medium** — store.go:66-117 — Two near-identical save functions exist. Keep one, `saveLocked`, and say in its name or comment that the caller must hold the lock.
- **low** — store.go:129-152 — Code generation is hard to follow. The loop breaks on success and returns on `i == 99`, and a separate "alias taken" check afterwards covers both aliases and the random-collision case. Clearer version: write `func (s *JSONStore) newCodeLocked() (string, error)` that loops 100 times and returns the first unused code, and keep the alias check in its own branch.
- **low** — store.go:218-224 — The hand-written O(n²) swap sort is slower and harder to read than `sort.Slice(links, func(i, j int) bool { return links[i].CreatedAt.Before(links[j].CreatedAt) })`. Map iteration order is random, so ties are unstable. Add a tie-break on `Code`.
- **low** — main.go:71 — The delete route condition slices the path by hand. Use `strings.HasPrefix` and `strings.TrimPrefix`, or Go 1.22+ `http.ServeMux` patterns (`"DELETE /api/links/{code}"`). That would also give a correct 405 for unsupported methods (handler_test.go:293-300 currently expects 404).
- **low** — store.go:296-299 — `rand.Seed` is deprecated and unnecessary since Go 1.20, so the `init` can go. `usedKeys` duplicates the keys of `links` and can be removed. Replace `regexp.MatchString` (compiled on every call, store.go:274) with a package-level `regexp.MustCompile`.
- **low** — main.go:33-48 — `Link` and `Store` live in `main.go` while the implementation is in `store.go`. Move them next to the store.
