# URL Shortener in Go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a concurrent, file-backed URL shortener service in Go with admin authentication and visit tracking

**Architecture:** HTTP server with in-memory sync.Map for concurrent access, periodic file persistence for crash safety, structured JSON storage format

**Tech Stack:** Go 1.24 standard library only (net/http, sync, encoding/json, crypto/rand, regexp, time)

**Spec:** Original specification in user request

## Global Constraints

- Go 1.24, standard library only (no third-party modules)
- Module name `shortener`
- Program in folder root: `go build -o shortener .` must work
- Storage: JSON file specified by `-data` flag, must survive restart
- Concurrency: correct behavior with many simultaneous requests
- Crash safety: never leave file server can't start from
- Admin token via `ADMIN_TOKEN` environment variable

## Review Focus

1. **Race condition on visit counts** - simultaneous GET requests should not lose increments
2. **File write failures during concurrent updates** - should not corrupt or lose data
3. **Admin token validation** - missing or wrong token should return 401 for admin endpoints
4. **Alias collision during concurrent creates** - no duplicate codes
5. **JSON file corruption on SIGKILL** - partial writes should not break startup

---

## File Structure

```
shortener/
├── main.go              # Entry point, flag parsing, server setup
├── store.go            # Data store interface with concurrency safety
├── store_file.go       # File persistence implementation
├── handlers.go         # HTTP handler functions
├── types.go           # Data types (Link, CreateRequest, etc.)
├── validation.go      # Input validation functions
└── handlers_test.go   # HTTP handler tests
```

## Task 1: Core Data Types and Validation

**Files:**
- Create: `types.go`
- Create: `validation.go`
- Test: `handlers_test.go` (partial)

**Interfaces:**
- Consumes: None
- Produces: 
  - `Link struct { Code, URL, CreatedAt, Visits }`
  - `CreateRequest struct { URL, Alias }`
  - `CreateResponse struct { Code, URL, CreatedAt, Visits }`
  - `ValidateURL(url string) error`
  - `ValidateAlias(alias string) error`

- [ ] **Step 1: Write failing tests for validation**

```go
func TestValidateURL(t *testing.T) {
    tests := []struct{
        name string
        url  string
        want error
    }{
        {"valid https", "https://example.com", nil},
        {"valid http", "http://example.com", nil},
        {"no scheme", "example.com", ErrInvalidURL},
        {"ftp scheme", "ftp://example.com", ErrInvalidURL},
        {"too long", "https://" + strings.Repeat("a", 2049), ErrInvalidURL},
        {"empty", "", ErrInvalidURL},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := ValidateURL(tt.url)
            if (got == nil) != (tt.want == nil) {
                t.Errorf("ValidateURL(%q) = %v, want %v", tt.url, got, tt.want)
            }
        })
    }
}

func TestValidateAlias(t *testing.T) {
    tests := []struct{
        name  string
        alias string
        want  error
    }{
        {"valid", "my-page-123", nil},
        {"too short", "ab", ErrInvalidAlias},
        {"too long", strings.Repeat("a", 33), ErrInvalidAlias},
        {"invalid chars", "my@page", ErrInvalidAlias},
        {"reserved api", "api", ErrInvalidAlias},
        {"empty", "", nil}, // empty is allowed (means generate)
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := ValidateAlias(tt.alias)
            if (got == nil) != (tt.want == nil) {
                t.Errorf("ValidateAlias(%q) = %v, want %v", tt.alias, got, tt.want)
            }
        })
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestValidate"`
Expected: FAIL with undefined functions/types

- [ ] **Step 3: Implement types and validation in `types.go` and `validation.go`**

Define Link, CreateRequest, CreateResponse structs with JSON tags.
Implement ValidateURL checking for http/https schemes, host presence, and max length.
Implement ValidateAlias using regexp for pattern and length checks.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run "TestValidate"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add types.go validation.go handlers_test.go
git commit -m "feat: add data types and validation"
```

## Task 2: Memory Store with Concurrency Safety

**Files:**
- Create: `store.go`
- Test: `store_test.go` (new file)

**Interfaces:**
- Consumes: Link type from Task 1
- Produces: 
  - `type Store interface { Create, Get, GetAll, Delete, IncrementVisits, Close }`
  - `type MemoryStore struct { data *sync.Map }`
  - `NewMemoryStore() *MemoryStore`

- [ ] **Step 1: Write failing tests for store operations**

```go
func TestMemoryStore_CreateAndGet(t *testing.T) {
    store := NewMemoryStore()
    defer store.Close()
    
    link := &Link{
        Code:      "abc123",
        URL:       "https://example.com",
        CreatedAt: time.Now(),
        Visits:    0,
    }
    
    // Test create
    err := store.Create(link)
    if err != nil {
        t.Fatalf("Create failed: %v", err)
    }
    
    // Test get
    got, err := store.Get("abc123")
    if err != nil {
        t.Fatalf("Get failed: %v", err)
    }
    if got.URL != link.URL {
        t.Errorf("Get() got URL = %v, want %v", got.URL, link.URL)
    }
    
    // Test duplicate create
    err = store.Create(link)
    if err == nil {
        t.Error("Expected error on duplicate create, got nil")
    }
}

func TestMemoryStore_IncrementVisitsConcurrent(t *testing.T) {
    store := NewMemoryStore()
    defer store.Close()
    
    link := &Link{
        Code:      "test",
        URL:       "https://example.com",
        CreatedAt: time.Now(),
        Visits:    0,
    }
    store.Create(link)
    
    // Concurrent increments
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            store.IncrementVisits("test")
        }()
    }
    wg.Wait()
    
    got, _ := store.Get("test")
    if got.Visits != 100 {
        t.Errorf("After 100 concurrent increments, Visits = %d, want 100", got.Visits)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestMemoryStore"`
Expected: FAIL with undefined types/functions

- [ ] **Step 3: Implement MemoryStore in `store.go`**

Use sync.Map for concurrent access. Implement Create, Get, GetAll, Delete, and IncrementVisits with atomic operations for visit counts.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run "TestMemoryStore"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add store.go store_test.go
git commit -m "feat: implement concurrent memory store"
```

## Task 3: File Persistence Layer

**Files:**
- Create: `store_file.go`
- Test: `store_file_test.go` (new file)

**Interfaces:**
- Consumes: Store interface from Task 2
- Produces:
  - `type FileStore struct { MemoryStore, filename, mu, dirty }`
  - `NewFileStore(filename string) (*FileStore, error)`
  - `LoadFromFile(filename string) (*FileStore, error)`

- [ ] **Step 1: Write failing tests for file persistence**

```go
func TestFileStore_Persistence(t *testing.T) {
    tempFile, err := os.CreateTemp("", "shortener-test-*.json")
    if err != nil {
        t.Fatal(err)
    }
    defer os.Remove(tempFile.Name())
    
    // Create store, add data
    store1, err := NewFileStore(tempFile.Name())
    if err != nil {
        t.Fatalf("NewFileStore failed: %v", err)
    }
    
    link := &Link{
        Code:      "persist",
        URL:       "https://example.com",
        CreatedAt: time.Now(),
        Visits:    5,
    }
    store1.Create(link)
    store1.IncrementVisits("persist") // Should trigger save
    store1.Close()
    
    // Load from same file
    store2, err := LoadFromFile(tempFile.Name())
    if err != nil {
        t.Fatalf("LoadFromFile failed: %v", err)
    }
    defer store2.Close()
    
    got, err := store2.Get("persist")
    if err != nil {
        t.Fatalf("Get after reload failed: %v", err)
    }
    if got.Visits != 6 {
        t.Errorf("Persisted Visits = %d, want 6", got.Visits)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestFileStore"`
Expected: FAIL with undefined types/functions

- [ ] **Step 3: Implement FileStore in `store_file.go`**

Embed MemoryStore, add filename field, mutex for file writes, dirty flag.
Override operations to trigger saves. Use atomic writes (write to temp file, rename) for crash safety.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run "TestFileStore"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add store_file.go store_file_test.go
git commit -m "feat: add file persistence with crash safety"
```

## Task 4: HTTP Handlers

**Files:**
- Create: `handlers.go`
- Modify: `handlers_test.go`

**Interfaces:**
- Consumes: Store interface, validation functions
- Produces:
  - `func handleCreateLink(store Store) http.HandlerFunc`
  - `func handleGetLink(store Store) http.HandlerFunc`
  - `func handleListLinks(store Store, adminToken string) http.HandlerFunc`
  - `func handleDeleteLink(store Store, adminToken string) http.HandlerFunc`

- [ ] **Step 1: Write failing tests for HTTP handlers**

```go
func TestHandleCreateLink(t *testing.T) {
    store := NewMemoryStore()
    defer store.Close()
    handler := handleCreateLink(store)
    
    reqBody := `{"url": "https://example.com"}`
    req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)
    
    if w.Code != http.StatusCreated {
        t.Errorf("Status code = %d, want %d", w.Code, http.StatusCreated)
    }
    
    var resp CreateResponse
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
        t.Fatalf("Failed to unmarshal response: %v", err)
    }
    if resp.URL != "https://example.com" {
        t.Errorf("Response URL = %s, want https://example.com", resp.URL)
    }
    if len(resp.Code) != 7 {
        t.Errorf("Generated code length = %d, want 7", len(resp.Code))
    }
}

func TestHandleCreateLinkInvalidURL(t *testing.T) {
    store := NewMemoryStore()
    defer store.Close()
    handler := handleCreateLink(store)
    
    reqBody := `{"url": "not-a-url"}`
    req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)
    
    if w.Code != http.StatusBadRequest {
        t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestHandleCreate"`
Expected: FAIL with undefined functions

- [ ] **Step 3: Implement HTTP handlers in `handlers.go`**

Implement handlers for all endpoints:
- Create link with validation and random code generation
- Get link with redirect and visit increment
- List links (admin only)
- Delete link (admin only)
- Admin auth middleware

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run "TestHandle"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add handlers.go handlers_test.go
git commit -m "feat: implement HTTP handlers"
```

## Task 5: Main Entry Point and Server Setup

**Files:**
- Create: `main.go`
- Create: `go.mod`

**Interfaces:**
- Consumes: FileStore, handlers from previous tasks
- Produces:
  - `func main()` with flag parsing
  - Server startup with graceful shutdown

- [ ] **Step 1: Write failing test for main behavior (integration test)**

```go
func TestServerIntegration(t *testing.T) {
    tempFile, err := os.CreateTemp("", "shortener-integration-*.json")
    if err != nil {
        t.Fatal(err)
    }
    defer os.Remove(tempFile.Name())
    
    // Start server in goroutine
    os.Setenv("ADMIN_TOKEN", "secret")
    go func() {
        os.Args = []string{"shortener", "-addr", "localhost:0", "-data", tempFile.Name()}
        main()
    }()
    time.Sleep(100 * time.Millisecond)
    
    // Test create link
    resp, err := http.Post("http://localhost:8080/api/links", 
        "application/json", 
        strings.NewReader(`{"url": "https://example.com"}`))
    if err != nil {
        t.Fatalf("POST failed: %v", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusCreated {
        t.Errorf("Create status = %d, want %d", resp.StatusCode, http.StatusCreated)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestServerIntegration"`
Expected: build error - missing main.go

- [ ] **Step 3: Implement main.go with flag parsing and server setup**

Parse -addr and -data flags.
Check ADMIN_TOKEN environment variable.
Load/create FileStore.
Set up HTTP routes with handlers.
Implement graceful shutdown.

- [ ] **Step 4: Create go.mod**

```bash
go mod init shortener
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./...`
Expected: All tests pass

Run: `go build -o shortener .`
Expected: Successful build

- [ ] **Step 6: Commit**

```bash
git add main.go go.mod
git commit -m "feat: main entry point and server setup"
```

## Task 6: Concurrency and Edge Case Tests

**Files:**
- Modify: `handlers_test.go`
- Modify: `store_test.go`

**Interfaces:**
- Consumes: All previous components
- Produces: Comprehensive concurrency tests

- [ ] **Step 1: Write failing concurrency tests**

```go
func TestConcurrentCreateWithSameAlias(t *testing.T) {
    store := NewMemoryStore()
    defer store.Close()
    
    link := &Link{
        Code:      "alias",
        URL:       "https://example.com",
        CreatedAt: time.Now(),
        Visits:    0,
    }
    
    var wg sync.WaitGroup
    errors := make(chan error, 10)
    
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            err := store.Create(link)
            if err != nil {
                errors <- err
            }
        }()
    }
    wg.Wait()
    close(errors)
    
    // Only one should succeed
    var errorCount int
    for range errors {
        errorCount++
    }
    if errorCount != 9 {
        t.Errorf("Expected 9 errors, got %d", errorCount)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail (or find issues)**

Run: `go test ./... -race -run "TestConcurrent"`
Expected: Tests may pass but race detector might find issues

- [ ] **Step 3: Fix any concurrency issues found**

Review and fix any race conditions or atomicity issues.

- [ ] **Step 4: Run tests with race detector**

Run: `go test ./... -race`
Expected: PASS with no race conditions detected

- [ ] **Step 5: Commit**

```bash
git add handlers_test.go store_test.go
git commit -m "test: add concurrency and edge case tests"
```

## Task 7: Final Integration and Verification

**Files:**
- All files

**Interfaces:**
- Final integration verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -race -v`
Expected: All tests pass with no races

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 3: Run build verification**

Run: `go build -o shortener .`
Expected: Successful build

Run: `./shortener -help`
Expected: Shows usage

- [ ] **Step 4: Manual test script verification**

Create a simple test script to verify API endpoints work correctly.

- [ ] **Step 5: Final commit with documentation**

```bash
git add README.md
git commit -m "docs: add README with usage instructions"
```

---

**Plan complete. Please review the plan. Which execution approach would you prefer?**

**Subagent-driven** - A fresh subagent implements each task and a fresh reviewer checks it before the next one starts, then a whole-branch review at the end. Most thorough; costs a fresh context per task and per review.

**Native** - I implement every task myself in this session, the way this harness runs work, then one fresh reviewer on the most capable model checks the whole branch. Cheapest and fastest; no independent review until the end.

**For this plan I recommend Native execution, because the tasks are sequential and build directly on each other's interfaces. The plan is detailed and specific, making it suitable for single-session implementation. A shipped mistake would be caught by the comprehensive test suite and race detector runs.**
