### Error handling
- **medium** — store.go:174-194, http.go:114-118 — Every redirect writes to disk. If the write fails, `Visit` returns an error and the client gets a 500 instead of the redirect. A full or read-only disk therefore takes down all redirects, even though the URL is already in memory. `TestStorageFailuresAndCorruption` (http_test.go:257) confirms that `GET /present` returns 500 when saving fails. — Count visits in memory and flush them to disk periodically or in the background. If you keep the synchronous save, log the failure and still redirect.
- **low** — store.go:121, 165-170, 187-192, 204-209 — If `dir.Sync()` fails after the rename, `saveLocked` returns `committed=true` with an error. The in-memory state is kept, but the caller gets a 500. For `Create`, the client sees a failure for a link that now exists and is persisted. A retry with the same alias then returns 409. — Treat a post-rename sync failure as success, and log it. Alternatively, return a distinct error and document it.
- **low** — store.go:181-183 — When the visit counter is exhausted, `Visit` returns a plain error, so the redirect fails with 500 forever. The value is unreachable in practice. — Stop incrementing at the maximum but still redirect.
- **low** — http.go:46 — The `Encode` error is discarded. A client that disconnects mid-response leaves no trace. — Log it at debug level, or leave a comment explaining why it is ignored.

### Secrets and sensitive data
- **low** — http.go:31, store.go:57 — `validURL` accepts URLs with embedded credentials, such as `https://user:pass@host/`. They are stored in plaintext in data.json and returned by `GET /api/links`. — Reject `u.User != nil`, or document that behaviour.
- **low** — store.go:102 — The temp file inherits `CreateTemp`'s 0600 mode, so data.json always ends up 0600 after a save. This is the safe default, but the README doesn't mention it. — Mention it in the README.

### Input validation
- **medium** — http.go:85-88, 126-171 — `POST /api/links` needs no authentication, and there is no rate limit and no cap on the number of links. Anyone can grow data.json without bound. Every later mutation rewrites the whole file and fsyncs it under the global lock, so the cost grows with each link. — Require the token for creation, or add a rate limit and a maximum link count. If open creation is intended, state that in the README.
- **low** — http.go:27 — The 2048 limit counts runes, not bytes. A URL of 2048 multi-byte characters can be several times larger in bytes. The 1 MiB body limit still bounds it. — Use `len(raw)` if the intent is a byte limit.

### External services
- **medium** — store.go:92-122, 174-194 — Each unauthenticated `GET /{code}` takes the global mutex, then serializes and writes the entire link set, runs two fsyncs (file and directory), and does a rename. Throughput is limited by disk sync latency, and the cost grows with the number of links. Anyone can hammer a single short link to keep the disk and lock busy, and the 30s `WriteTimeout` can be reached under load. — Batch or defer visit persistence (see Error handling). If the design must stay synchronous, say so in the README, since "saved before responding" is a deliberate promise.
- **low** — main.go:49 — Compares `err != http.ErrServerClosed` directly. — Use `errors.Is`.

### Resources and concurrency
- **low** — store.go:101-108 — `defer dir.Close()`, `defer f.Close()` and `defer os.Remove(name)` all run, and `f.Close()` is called twice on the success path. The second close is harmless and its error is ignored, but the pattern is confusing. After a successful rename, `os.Remove` fails silently, which is correct. — Add a comment, or close once in a small helper.
- **low** — store.go:102-107 — A process killed between `CreateTemp` and `Rename` leaves `.shortener-*.tmp` files behind. They are never cleaned up. The README says they are "not used", but not that they accumulate. — Remove stale `.shortener-*.tmp` files in `openStore`.
- No data races found. All store access goes through `mu`, and `List` returns a copy.

### Tests
- **medium** — http_test.go:243-252 — The rollback test points `s.path` at a missing directory. The failure happens at `os.Open(dir)`, before any temp file is created. The failures after that point are never tested: write, sync, rename, and the `committed=true` directory-sync case. The `committed` branch logic in `Create`, `Visit` and `Delete` therefore has no test. — Inject a failure, for example by making a file's directory read-only, or by adding a small `syncDir`/`rename` hook. Assert the in-memory state for both the committed and uncommitted outcomes.
- **low** — store.go:181 — The counter-exhausted path has no test. — Add one that sets `Visits` to `MaxUint64`.
- **low** — persistence_test.go:14-55 — The atomic-replacement test can only fail if its reader happens to hit a window of partial data. It also never checks that temp files are removed after a save or a failed save. — Add an assertion that the directory contains only data.json after the operations.
- **low** — main.go:15-57 — There is no test of graceful shutdown, or of the listen-failure path that releases the goroutine. Only the missing-token exit is tested (persistence_test.go:91). — Add a test that runs the server on an in-use port and checks that `run()` returns an error instead of hanging.
- **low** — http_test.go:128, 175 — Layout like `struct { method, path string; status int }` and the misaligned struct fields suggests gofmt wasn't run on the test file. This is only a cleanliness point.

### Clarity
- **low** — http.go:96 — This single `case` condition trims the same prefix twice, checks the length, and checks for a slash. In plain terms, it means "the path is `/api/links/<id>` and `<id>` is non-empty and has no slash". — Compute `id, ok := strings.CutPrefix(path, "/api/links/")`
