### Error handling
- **high** — store.go:175 — When `save()` fails in `Delete`, the rollback puts back `&Link{Code: code}`, not the original link. The URL, creation time and visit count are lost, and the in-memory entry is now an empty link. A later redirect to that code would send `Location: ""`. Keep the original pointer (`link := s.links[code]`) and restore that. [verified] A scratch test with a failing save left `{Code:abc URL:"" Visits:0}` after a failed delete, where the original had 1 visit.
- **medium** — handler.go:118-121 — The redirect is refused with a 500 if the visit counter can't be saved. A disk-full or permission error on the data file stops every short link from working, even though the link was found. Log the `Visit` error and still send the redirect. [verified] A failed save makes `Visit` return an error (`open …/d.json.tmp: no such file`), and the handler turns that into a 500.
- **medium** — handler.go:52-56 — Any `io.ReadAll` error is reported as `413 request body too large`. A client disconnect or timeout gets the same answer. Check for `*http.MaxBytesError` with `errors.As` and return 400 for other errors. A 2 MiB body does return 413 [verified].
- **medium** — handler.go:75-83, handler.go:160 — Errors are classified by comparing `err.Error()` to strings. Changing a message in store.go quietly turns a 400 or 409 into a 500. The long list at line 78 must be kept identical to the messages at store.go:107, 196-226. Define sentinel errors (`ErrAliasTaken`, `ErrNotFound`, and a `ValidationError` type) and match with `errors.Is` or `errors.As`.
- **low** — handler.go:102, 139, 187 — The `json.NewEncoder(w).Encode` errors are ignored. By then the status is already sent, so this only loses a log line. Log the error.
- **low** — store.go:83-88 — `save` does not `fsync` the temp file before `Rename`. A power loss can leave an empty or truncated data file, which contradicts the README's "supports crash recovery" (README.md:92). A failed rename also leaves a stale `.tmp` file behind. Open the file, write, `Sync`, `Close`, then rename, and remove the temp file on error.
- **low** — store.go:63-67 — `load` silently drops earlier entries if the file has duplicate codes. It also doesn't check loaded entries. Reject duplicates or log them.

### Secrets and sensitive data
- **medium** — handler.go:182 — The admin token is compared with `==`, which is not constant-time and could leak the token through timing. Use `subtle.ConstantTimeCompare` on `[]byte` values (or on hashes of them).
- **low** — store.go:84 — The data file is created with mode 0644, so any local user can read it. It holds only links, but 0600 is safer. The token itself is never logged. I found no secret leaks.

### Input validation
- **medium** — store.go:194-213 — `validateURL` accepts any `http` or `https` URL, including `http://localhost/...`, private IPs, and URLs with embedded credentials (`https://user:pass@host`). The service will issue redirects to them, and the link is public. Decide on a policy, for example rejecting `u.User != nil`, and document it.
- **low** — handler.go:63 — `json.Unmarshal` accepts unknown fields and trailing data. A typo like `"alais"` is silently ignored. Use a `json.Decoder` with `DisallowUnknownFields` if stricter input is wanted.
- **low** — handler.go:36-39, 87 — The create response sets `Location: /api/links/{code}`, but a `GET` on that path returns 401 for anonymous callers and 405 with the admin token [verified]. The header points at a URL that doesn't work. Either add a GET handler or drop the header.
- **low** — handler.go:36-39 — `/api/links/...` requests are always sent to the delete handler. The auth check runs before the method check, so a wrong method without a token gets 401, not 405. This is a minor inconsistency. Routing on method and path first would be clearer.
- **low** — handler.go:38 — `r.URL.Path != "/api/links"` is dead code, because an earlier case already handles that path. `HEAD /abc` returns 404 [verified]. Redirect handling could accept HEAD.

### External services
- **medium** — main.go:28 — `http.ListenAndServe` uses a server with no `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` or `IdleTimeout`. Slow clients (slowloris) can hold connections open indefinitely. Build an `http.Server` with timeouts and add graceful shutdown on SIGINT/SIGTERM.
- **medium** — handler.go:32 — `POST /api/links` needs no authentication or rate limit. Anyone can fill the data file with links. Each creation rewrites the entire file under the global lock. Add a rate limit or a cap on the number of links.
- **medium** — store.go:154-156 — Every redirect (`Visit`) rewrites the whole JSON file while holding the write lock. Cost grows with the number of links, and all requests, including reads, queue behind the disk write. Batch or periodically flush the counters, or keep them in memory and persist them in the background.

### Resources and concurrency
- **low** — handler.go:15-19 — `Handler.mu` is never used. It suggests the handler does its own locking when it doesn't. Remove it.
- **low** — store.go:134-143, handler.go:112-123 — `Get` returns the store's internal `*Link`, and the handler uses it after the lock is released. Today it only reads `URL`, which never changes, so there is no race now. If any code reads or writes `Visits` through that pointer it will race with `Visit`. Return a copy (`Link`) instead.
- **low** — store.go:64-66 — The `link := links[i]` copy is correct, so there is no loop-variable bug. I found no leaked files or response bodies.

### Tests
- **high** — store_test.go, handler_test.go — Nothing tests that a failed `save` rolls back correctly. The `Delete` bug at store.go:175 therefore went unnoticed. The `mockStore` can't fail at all. Add tests that make the save fail (for example, point `filePath` at a missing directory) for `Create`, `Visit` and `Delete`.
- **medium** — handler_test.go:26-91 — `mockStore` returns `"generated"` for every un-aliased create and never validates input. Handler tests for 400 and 409 mapping through error strings (handler.go:75-83) can't catch a mismatch with the real store messages. Run the handler tests against `JSONStore` in a temp directory, or move to sentinel errors.
- **low** — test_run.sh:8 — The script passes `-addr:0`, which is not a valid flag form (it should be `-addr :0`). `flag.Parse` would fail before the `ADMIN_TOKEN` check, so the grep for "ADMIN_TOKEN" probably fails. I didn't run this script. The script also leaves a built binary in the repo and isn't part of `go test`.
- **low** — handler_test.go:395, store_test.go:165 — The concurrency tests only help when run with `-race`. The README (README.md:97-105) doesn't mention `-race`. Add it to the test instructions.
- **low** — handler_test.go — I saw no tests for these cases: a failing store returning 500, `HEAD` or unsupported methods on `/{code}`, and a non-Bearer `Authorization` header. I didn't read every test, so check these before adding them.

### Clarity
- **medium** — handler.go:75-83 — This is one `switch err.Error()` with a long list of message strings. In plain English: it turns an error's text into an HTTP status. It's hard to read and easy to break. Clearer version: `var ve *ValidationError; switch { case errors.Is(err, ErrAliasTaken): 409; case errors.As(err, &ve): 400; default: 500 }`.
- **low** — handler.go:28-43 — The router is a `switch` on method and path mixed with `strings.HasPrefix`. The `/api/links/` case ignores the method and the redirect case has a redundant path check. Check the path first, then the method inside each branch, with one `405` fallback per path. Alternatively, use Go 1.22 `ServeMux` patterns (`"POST /api/links"`, `"DELETE /api/links/{code}"`, `"GET /{code}"`).
- **low** — handler.go:171-183 — `isAdmin` has two early returns that are covered by the third. It can be written as `token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); return ok && constantTimeEqual(token, h.adminToken)`.
- **low** — utils.go:9-14 — The const block is not aligned (gofmt would fix it). `codeCharsLen` is computed from a `const` string and then converted back to `int64` at line 34. That's fine, but `len(codeChars)` used directly would be simpler.
