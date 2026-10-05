### Error handling
- **medium** — store.go:154-167 and main.go:96-104 — A redirect depends on the visit counter being saved to disk. If `saveLocked` fails, for example on a full disk or a missing directory, `Visit` rolls back the count and returns an error. The handler then answers 500 and never redirects. A bookkeeping failure takes down the main function of the service. I removed the data directory and requested an existing link: it returned 500 instead of redirecting. [verified] — Make the redirect succeed regardless. Increment in memory, return the URL, and persist the counter best-effort (log the error, or flush periodically).
- **medium** — main.go:76-83, 96-104, 145-152 — Every 500 path discards the underlying error and returns only "internal error". Nothing is logged, so a disk failure, as in the test above, is invisible to the operator. — Call `log.Printf("...: %v", err)` before writing the 500.
- **low** — main.go:28 — `json.NewEncoder(w).Encode(v)` ignores its error. — Log it. It only fails when the client has gone away or `v` is unmarshalable, so this is minor.
- **low** — store.go:56-58 — When loading, a duplicate `code` in the file is silently skipped, so one of the two records is dropped. The next save then permanently erases it. — Return an error, or at least log the skipped code.
- **low** — store.go:101-104 — The error from the directory `Sync` is ignored. The rename has already happened, so the write counts as successful. — This is acceptable as best-effort, but add a comment saying so.

### Secrets and sensitive data
- **low** — main.go:157-169 — `validateURL` accepts URLs containing credentials (`https://user:pass@host/`). Anyone with the admin token sees them through `GET /api/links`, and the 302 hands them to every visitor. They are also stored in plaintext in data.json. — Reject URLs where `u.User != nil`.
- **low** — main.go:175, 41 — The admin token is a single static env var and is compared in constant time, which is fine. There is no protection against brute-forcing it: failed attempts are not rate-limited or logged. — Use a long random token, and consider rate limiting or logging failed attempts.

### Input validation
- **medium** — main.go:47-55 — `POST /api/links` needs no authentication, so anyone can create unlimited links. Each create rewrites the whole data file and grows memory. The service can be filled with spam or phishing redirects and its disk exhausted. — Require auth for creation, or add per-IP rate limiting and a cap on the number of links. If creation is meant to be public, say so in a comment.
- **low** — store.go:47-61 — The data file's contents are trusted on load. Records with empty codes, codes that don't match `aliasRe`, or a code of `api` are accepted, and nothing re-validates the URLs. A hand-edited file could create entries that can never be reached or that bypass the checks. — Validate each record with the same rules used at creation.
- **low** — main.go:127 — The body is already fully read and size-limited, then copied via `string(body)` into a new reader. — Use `bytes.NewReader(body)`. See Clarity.

### External services
- **medium** — main.go:185 — `http.ListenAndServe` uses a default server with no `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` or `IdleTimeout`. Slow clients can hold connections open indefinitely (Slowloris). — Build an `&http.Server{...}` with those timeouts set, and ideally add graceful shutdown.
- **low** — main.go:185 — There is no graceful shutdown. A SIGTERM during a request ends the process. The atomic rename in `saveLocked` protects the file itself, but in-flight requests get dropped. — Use `Server.Shutdown` on a signal.

### Resources and concurrency
- **medium** — store.go:154-167 — Every redirect marshals the whole dataset, writes a temp file, calls `fsync` twice (file and directory) and renames it, all while holding the global mutex. Throughput is limited by disk sync latency, and all requests, including reads of `List`, queue behind it. Combined with the open `POST`, this makes DoS easy. — Keep counters in memory and flush them in a background goroutine on an interval or at shutdown. Write only when data changes.
- **low** — store.go:129-138 — The loop that picks a random code has no retry limit and runs under the lock. In practice 62^7 codes make this safe, but a crypto/rand failure is the only exit. — Cap the attempts (for example 10) and return an error.
- **low** — main.go:91-96 — A HEAD request counts as a visit and triggers a disk write. Link checkers and crawlers inflate the counts. — Skip the increment for HEAD.

### Tests
- **medium** — main_test.go — No test covers the failure paths in `store.go`: write errors and the rollback logic in `Create`, `Visit` and `Delete`. This is where the 500-on-redirect behaviour comes from. — Add tests that make the data directory unwritable and assert both the response and that the in-memory state is unchanged.
- **low** — main_test.go:31-82 — `TestFlow` is one long sequence and mostly calls `t.Fatal()` with no message. Failures won't say which step broke. The following are missing:
  - `DELETE` without a token or with a wrong token (401).
  - A wrong HTTP method on `/api/links/x` and on `/x`.
  - `HEAD`.
  - A request body with trailing data (`{...}{...}`), which main.go:133 is meant to reject.
  - A corrupt data file in `OpenStore`.
  — Split into table-driven subtests with descriptive messages.
- **low** — main_test.go:69 — The `json.Unmarshal` error is ignored, so a malformed response would show up only as a confusing length mismatch. — Check the error.
- **low** — main_test.go:84-103 — `TestConcurrent` only gives value under `-race`, and it never checks the persisted file. — Run CI with `-race`, and reopen the store afterwards to confirm the counts persisted.

### Clarity
- **low** — main.go:127 — `json.NewDecoder(strings.NewReader(string(body)))` converts the bytes to a string and back into a reader. — Use `bytes.NewReader(body)`, or decode straight from `http.MaxBytesReader` and drop `io.ReadAll`.
- **low** — main.go:133 — `if _, err := dec.Token(); err != io.EOF` is a trick for rejecting trailing data. It is hard to read and compares the error with `!=`. — Add a comment ("reject anything after the first JSON value") and use `errors.Is`, or use `dec.More()`.
- **low** — store.go:176-185 — `Delete` copies the order slice, removes the entry and restores the copy on failure. Because it relies on `append` shifting in place, it needs the copy to be safe. — Save first with a newly built slice, or use `slices.Delete` on a copy. Then only assign the new state on success.
- **low** — main.go:141 and store.go:134 — The "api is reserved" rule is repeated in three places: main.go:87, main.go:141 and store.go:134. — Put it in one `reserved(code)` helper.
