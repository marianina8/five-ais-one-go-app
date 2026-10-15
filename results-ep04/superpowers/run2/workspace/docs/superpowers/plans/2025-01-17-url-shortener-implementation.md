# URL Shortener Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-ready URL shortener service in Go with persistent JSON storage, admin authentication, and concurrent-safe operations.

**Architecture:** HTTP server using net/http package with in-memory store backed by JSON file persistence, using mutex-based synchronization for thread safety. Service validates URLs, generates unique codes, tracks visits, and provides admin API.

**Tech Stack:** Go 1.24 standard library only (no third-party modules)

**Spec:** User request specification in chat

## Global Constraints

- Go 1.24, standard library only (no third-party modules; the network is off)
- Module name `shortener`
- Program in folder root: `go build -o shortener .` must work
- Own tests (`go test ./...`)
- ADMIN_TOKEN environment variable required
- Command line flags: `-addr :8080` (default), `-data data.json` (default)
- JSON file persistence with atomic writes
- Thread-safe concurrent operations

## Review Focus

1. **Duplicate alias generation on concurrent requests** - Multiple goroutines could generate same random code
2. **Race condition on visit counting** - Increment not atomic with save
3. **JSON file corruption on crash** - Write not atomic
4. **Admin token verification missing on DELETE** - Authorization check skipped
5. **URL validation incomplete** - Missing host validation or scheme checking

---

## File Structure

**Root files:**
- `go.mod` - Module definition
- `main.go` - CLI parsing, server startup, admin token validation
- `server.go` - HTTP server setup and routing

**Internal package (`internal/`):**
- `internal/store/store.go` - Thread-safe store with file persistence
- `internal/store/json_store.go` - JSON serialization/deserialization
- `internal/models/models.go` - Data structures (Link, error response)
- `internal/handlers/handlers.go` - HTTP handlers for all endpoints
- `internal/handlers/auth.go` - Admin authentication middleware
- `internal/handlers/validation.go` - Request validation helpers

**Tests:**
- `internal/store/store_test.go` - Store unit tests
- `internal/handlers/handlers_test.go` - Handler integration tests
- `cmd/main_test.go` - CLI and startup tests (if needed)

### Task 1: Project Setup and Data Models

**Files:**
- Create: `go.mod`
- Create: `internal/models/models.go`
- Create: `internal/store/json_store.go` (minimal for now)

**Interfaces:**
- Consumes: Nothing (foundation)
- Produces: 
  - `models.Link` struct with fields: `Code`, `URL`, `Alias`, `CreatedAt` (time.Time), `Visits` (int)
  - `models.ErrorResponse` struct with `Error` field
  - `models.CreateRequest` struct with `URL` and `Alias` fields
  - `JSONStore` struct with `Links` map and `filePath` field

- [ ] **Step 1: Create go.mod**

```bash
go mod init shortener
```

- [ ] **Step 2: Initialize models package**

Create `internal/models/models.go` with struct definitions matching spec:
- `Link` with all required fields
- `ErrorResponse` with `Error string` field
- `CreateRequest` with `URL` and `Alias` fields (both strings, `Alias` optional)
- JSON tags for all fields

- [ ] **Step 3: Initialize JSON store structure**

Create `internal/store/json_store.go` with:
- `JSONStore` struct for persistence
- `Load()` and `Save()` method stubs
- Test that it compiles

- [ ] **Step 4: Run go mod tidy**

```bash
go mod tidy
```

- [ ] **Step 5: Commit**

```bash
git add go.mod internal/
git commit -m "feat: initial project setup and data models"
```

### Task 2: Thread-Safe Store Implementation

**Files:**
- Create: `internal/store/store.go`
- Modify: `internal/store/json_store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `models.Link` from Task 1
- Produces:
  - `Store` interface with methods: `Create`, `Get`, `Delete`, `IncrementVisits`, `ListAll`, `Save`
  - `FileStore` concrete implementation with mutex protection

- [ ] **Step 1: Write failing Store interface test**

In `store_test.go`, define test for `Store.Create()` method that expects thread-safe operation.

- [ ] **Step 2: Verify test fails**

Run: `go test ./internal/store -run TestStore`
Expected: compilation failure (interface not defined)

- [ ] **Step 3: Implement Store interface in `store.go`**

Define `Store` interface with all required methods:
- `Create(url, alias string) (*models.Link, error)`
- `Get(code string) (*models.Link, error)`  
- `Delete(code string) error`
- `IncrementVisits(code string) error`
- `ListAll() []models.Link`
- `Save() error`

- [ ] **Step 4: Implement FileStore in `store.go`**

Create `FileStore` struct with:
- `mu sync.RWMutex` for thread safety
- `links map[string]models.Link`
- `jsonStore` field for persistence
- All interface methods with mutex locking

- [ ] **Step 5: Complete JSON store implementation**

Update `json_store.go`:
- Implement `Load()` reading from file
- Implement `Save()` writing atomically (temp file + rename)
- Handle file creation on first start

- [ ] **Step 6: Run tests**

Run: `go test ./internal/store -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/
git commit -m "feat: thread-safe store with file persistence"
```

### Task 3: Request Validation and URL Generation

**Files:**
- Create: `internal/handlers/validation.go`
- Modify: `internal/store/store.go` (add random code generation)
- Test: `internal/store/store_test.go` (validation tests)

**Interfaces:**
- Consumes: `Store` interface from Task 2
- Produces:
  - `GenerateRandomCode(length int) string` function
  - Validation functions: `ValidateURL`, `ValidateAlias`
  - Constants for: max URL length (2048), code length (7), alias regex

- [ ] **Step 1: Write validation tests**

Add tests for:
- `ValidateURL` rejects non-http(s) URLs
- `ValidateURL` rejects URLs without host
- `ValidateURL` rejects URLs > 2048 chars
- `ValidateAlias` rejects invalid patterns
- `ValidateAlias` rejects "api" alias
- `GenerateRandomCode` produces correct format

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/store -run TestValidation`
Expected: FAIL (functions not defined)

- [ ] **Step 3: Implement validation functions**

In `validation.go`:
- `ValidateURL(url string) error` - checks scheme, host, length
- `ValidateAlias(alias string) error` - regex match, not "api"
- Constants for constraints from spec

- [ ] **Step 4: Implement random code generation**

In `store.go`:
- `GenerateRandomCode(length int) string` - using `math/rand` with seeding
- Ensure uniqueness check in `Create` method

- [ ] **Step 5: Update Store.Create() method**

Add validation calls and random code generation logic:
1. Validate URL
2. If alias provided, validate it
3. Check for duplicate alias/code
4. Generate random code if no alias
5. Create Link with zero visits and current time

- [ ] **Step 6: Run tests**

Run: `go test ./internal/store -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/
git commit -m "feat: URL validation and random code generation"
```

### Task 4: HTTP Handlers Foundation

**Files:**
- Create: `internal/handlers/handlers.go`
- Create: `internal/handlers/auth.go` (stub)
- Test: `internal/handlers/handlers_test.go`

**Interfaces:**
- Consumes: `Store` interface, validation functions
- Produces:
  - Handler functions: `CreateLink`, `GetLink`, `ListLinks`, `DeleteLink`
  - Response helpers: `jsonResponse`, `errorResponse`
  - Authentication middleware stub

- [ ] **Step 1: Write handler tests**

Add tests for:
- `CreateLink` handler returns 201 with correct JSON
- `CreateLink` validates request body size (1 MiB limit)
- `GetLink` returns 302 redirect
- Error responses match `{"error": "message"}` format

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/handlers -run TestHandlers`
Expected: FAIL (handlers not defined)

- [ ] **Step 3: Implement response helpers**

In `handlers.go`:
- `jsonResponse(w http.ResponseWriter, status int, data any)`
- `errorResponse(w http.ResponseWriter, status int, message string)`
- Set proper Content-Type headers

- [ ] **Step 4: Implement CreateLink handler**

Handler logic:
1. Parse and validate JSON (malformed: 400)
2. Check body size (over 1 MiB: 413)
3. Call store.Create()
4. Handle conflicts (409), validation errors (400)
5. Return 201 with link data

- [ ] **Step 5: Implement GetLink handler**

Handler logic:
1. Extract code from path
2. Call store.Get()
3. If found: increment visits, return 302 redirect
4. If not found: 404 error

- [ ] **Step 6: Run tests**

Run: `go test ./internal/handlers -v`
Expected: PASS for implemented handlers

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/
git commit -m "feat: HTTP handlers for create and redirect"
```

### Task 5: Admin Authentication and API

**Files:**
- Modify: `internal/handlers/auth.go`
- Modify: `internal/handlers/handlers.go`
- Test: `internal/handlers/handlers_test.go` (auth tests)

**Interfaces:**
- Consumes: `Store` interface
- Produces:
  - `adminMiddleware(next http.Handler) http.Handler`
  - `ListLinks` and `DeleteLink` handlers
  - Proper Authorization header extraction

- [ ] **Step 1: Write auth tests**

Add tests for:
- Admin middleware rejects missing token (401)
- Admin middleware rejects wrong token (401)
- Admin middleware passes correct token
- `DeleteLink` returns 204 on success
- `ListLinks` returns array oldest first

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/handlers -run TestAuth`
Expected: FAIL (middleware not defined)

- [ ] **Step 3: Implement admin middleware**

In `auth.go`:
- `adminMiddleware` validates `Authorization: Bearer <token>`
- Compare with environment variable `ADMIN_TOKEN`
- Return 401 on failure, call next on success

- [ ] **Step 4: Implement ListLinks handler**

In `handlers.go`:
- Wrap with admin middleware
- Call store.ListAll()
- Return JSON array sorted by CreatedAt ascending

- [ ] **Step 5: Implement DeleteLink handler**

In `handlers.go`:
- Wrap with admin middleware  
- Extract code from path
- Call store.Delete()
- Return 204 on success, 404 if not found

- [ ] **Step 6: Add HEAD method support**

Update GetLink handler to handle HEAD requests per spec

- [ ] **Step 7: Run tests**

Run: `go test ./internal/handlers -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/handlers/
git commit -m "feat: admin authentication and API endpoints"
```

### Task 6: Server Setup and CLI

**Files:**
- Create: `main.go`
- Create: `server.go`
- Test: `cmd/` (optional integration tests)

**Interfaces:**
- Consumes: All handlers, store
- Produces:
  - CLI flag parsing for `-addr` and `-data`
  - Server startup with routes
  - Admin token validation on startup

- [ ] **Step 1: Write main.go**

Implement:
1. Parse `-addr` and `-data` flags
2. Check `ADMIN_TOKEN` environment variable (exit if empty)
3. Initialize store with data file path
4. Load existing data
5. Start HTTP server with routes

- [ ] **Step 2: Write server.go**

Implement:
1. Route definitions:
   - `POST /api/links` → CreateLink
   - `GET /api/links` → ListLinks (with admin middleware)
   - `DELETE /api/links/{code}` → DeleteLink (with admin middleware)
   - `GET /{code}` → GetLink
   - `HEAD /{code}` → GetLink
2. 405 Method Not Allowed for other methods on known paths
3. Global middleware for JSON error responses

- [ ] **Step 3: Test CLI flags**

Build and test with different flag combinations:
```bash
go build -o shortener .
ADMIN_TOKEN=secret ./shortener -help
```

- [ ] **Step 4: Verify admin token requirement**

Test that missing ADMIN_TOKEN exits with error:
```bash
./shortener
```
Expected: error and non-zero exit code

- [ ] **Step 5: Test full integration**

Start server and test basic workflow with curl:
1. Create link
2. Redirect works
3. Admin endpoints with token

- [ ] **Step 6: Run all tests**

Run: `go test ./... -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add main.go server.go
git commit -m "feat: CLI and server setup"
```

### Task 7: Concurrency and Edge Case Testing

**Files:**
- Modify: `internal/store/store_test.go` (concurrency tests)
- Modify: `internal/handlers/handlers_test.go` (edge cases)
- Test: Add Review Focus tests

**Interfaces:**
- Consumes: All existing code
- Produces: Additional test coverage for spec requirements

- [ ] **Step 1: Add concurrency tests for store**

Test concurrent operations:
- Multiple goroutines creating links simultaneously
- Visit counting race condition prevention
- File save operations under load

- [ ] **Step 2: Add Review Focus tests**

Implement tests for each Review Focus item:
1. Test duplicate code generation prevention
2. Test atomic visit increment with save
3. Test JSON file atomic writes (simulate crash)
4. Test admin auth on all protected endpoints
5. Test comprehensive URL validation

- [ ] **Step 3: Test body size limits**

Add test for 1 MiB request body limit:
- Create test with body just under limit (should pass)
- Create test with body over limit (should get 413)

- [ ] **Step 4: Test alias validation**

Add tests for:
- "api" alias rejection
- Regex pattern validation
- Minimum (3) and maximum (32) length

- [ ] **Step 5: Run race detector**

Run: `go test ./... -race`
Expected: No race conditions detected

- [ ] **Step 6: Run go vet**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 7: Final build test**

Run: `go build -o shortener .`
Expected: Successful build

- [ ] **Step 8: Commit**

```bash
git add internal/store/store_test.go internal/handlers/handlers_test.go
git commit -m "test: comprehensive concurrency and edge case coverage"
```

### Task 8: Documentation and Final Verification

**Files:**
- Create: `README.md`
- Create: `docs/` (optional usage examples)
- Verify: All spec requirements

**Interfaces:**
- Consumes: Completed codebase
- Produces: Documentation and verification report

- [ ] **Step 1: Create README.md**

Document:
- Installation and building
- Running instructions with examples
- API documentation
- Environment variables
- Command line flags

- [ ] **Step 2: Verify all spec requirements**

Checklist against spec:
- [x] POST /api/links with validation ✓
- [x] GET /{code} with 302 redirect and visit counting ✓  
- [x] GET /api/links with admin auth ✓
- [x] DELETE /api/links/{code} with admin auth ✓
- [x] JSON file persistence ✓
- [x] Atomic saves before response ✓
- [x] Thread-safe operations ✓
- [x] Admin token env variable ✓
- [x] CLI flags ✓
- [x] Error responses in JSON format ✓

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: All tests pass

- [ ] **Step 4: Run go vet**

Run: `go vet ./...`
Expected: Clean

- [ ] **Step 5: Create example usage**

Add example curl commands to README:
- Creating a link
- Following a link
- Listing links (admin)
- Deleting a link (admin)

- [ ] **Step 6: Final commit**

```bash
git add README.md
git commit -m "docs: add README and verify all requirements"
```

---

**Plan complete and saved to `docs/superpowers/plans/2025-01-17-url-shortener-implementation.md`. Please review the plan. Which execution approach would you prefer?**

- **Subagent-driven** - A fresh subagent implements each task and a fresh reviewer checks it before the next one starts, then a whole-branch review at the end. Most thorough; costs a fresh context per task and per review.
- **Native** - I implement every task myself in this session, the way this harness runs work, then one fresh reviewer on the most capable model checks the whole branch. Cheapest and fastest; no independent review until the end. Runs well with a mid-tier session model, since the plan carries the design.

**For this plan I recommend Native execution, because the tasks have clear dependencies and interfaces are well-defined in the plan, making sequential implementation straightforward. The plan is comprehensive and leaves minimal ambiguity for an experienced Go developer. Does the plan capture what you want, and which approach should we use?**