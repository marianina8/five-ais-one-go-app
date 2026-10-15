# URL Shortener Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go URL shortener service that stores links in a JSON file, handles concurrent requests safely, and provides APIs for creating, viewing, listing, and deleting links.

**Architecture:** Use gorilla/mux-style routing with http.ServeMux; store data in a JSON file with atomic writes; use mutexes for thread-safe access; generate random codes for short URLs; validate API requests and admin auth.

**Tech Stack:** Go 1.24, standard library only

**Spec:** Build a URL shortener in Go (provided in user prompt)

## Global Constraints

- Go 1.24, standard library only (no third-party modules)
- Module name: `shortener`
- Program in folder root: `go build -o shortener .` must work
- Tests: `go test ./...`
- Network is off

## Review Focus

1. Race conditions with concurrent visits counting
2. Atomic writes preventing data corruption on crashes
3. Duplicate code generation collisions
4. Admin token validation for secure endpoints
5. Proper HTTP status codes and JSON error responses

---

### Task 1: Project Setup and Data Structures

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `link.go`
- Create: `store.go`
- Create: `store_test.go`

**Interfaces:**
- Consumes: None (starting point)
- Produces: 
  - `type Link struct`: definition with fields
  - `type Store interface`: CRUD operations
  - `makeStore()` function signature

- [ ] **Step 1: Create go.mod file**

```go
module shortener

go 1.24
```

- [ ] **Step 2: Create main.go with basic structure**

```go
package main

import (
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
)

func main() {
    addr := flag.String("addr", ":8080", "HTTP server address")
    dataPath := flag.String("data", "data.json", "Path to data file")
    flag.Parse()

    if os.Getenv("ADMIN_TOKEN") == "" {
        log.Fatal("ADMIN_TOKEN environment variable is required")
    }

    fmt.Printf("Starting server on %s\n", *addr)
    log.Fatal(http.ListenAndServe(*addr, nil))
}
```

- [ ] **Step 3: Run to verify setup**

Run: `go build -o shortener .`
Expect: Successful build

- [ ] **Step 4: Create link.go with Link struct**

```go
package main

import "time"

type Link struct {
    Code      string    `json:"code"`
    URL       string    `json:"url"`
    Alias     string    `json:"alias,omitempty"`
    CreatedAt time.Time `json:"created_at"`
    Visits    int       `json:"visits"`
}
```

- [ ] **Step 5: Create store.go with Store interface**

```go
package main

import "sync"

type Store interface {
    Create(link Link) error
    Get(code string) (Link, bool)
    GetAll() []Link
    IncrementVisits(code string) error
    Delete(code string) error
}

type fileStore struct {
    mu   sync.RWMutex
    path string
    data map[string]Link
}
```

- [ ] **Step 6: Commit**

```bash
git add go.mod main.go link.go store.go
git commit -m "feat: setup project structure and data types"
```

### Task 2: JSON File Store Implementation

**Files:**
- Modify: `store.go` (add implementation)
- Modify: `store_test.go` (create tests)

**Interfaces:**
- Consumes: `type Link struct` from Task 1
- Produces:
  - `func NewFileStore(path string) (Store, error)`: creates store
  - `load()` and `save()` methods for atomic operations

- [ ] **Step 1: Write test for store creation and loading**

```go
package main

import (
    "os"
    "testing"
)

func TestNewFileStore(t *testing.T) {
    path := "test_data.json"
    defer os.Remove(path)

    store, err := NewFileStore(path)
    if err != nil {
        t.Fatalf("Failed to create store: %v", err)
    }
    defer store.(*fileStore).close()

    if store == nil {
        t.Fatal("Store should not be nil")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v -run TestNewFileStore`
Expect: FAIL with "function not defined"

- [ ] **Step 3: Implement NewFileStore function in store.go**

```go
func NewFileStore(path string) (Store, error) {
    fs := &fileStore{
        path: path,
        data: make(map[string]Link),
    }
    if err := fs.load(); err != nil {
        return nil, err
    }
    return fs, nil
}
```

- [ ] **Step 4: Implement load and save methods**

```go
func (fs *fileStore) load() error {
    // Load JSON file, create if doesn't exist
    // Use atomic file operations
}

func (fs *fileStore) save() error {
    // Save with atomic write (write to temp file, then rename)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v -run TestNewFileStore`
Expect: PASS

- [ ] **Step 6: Commit**

```bash
git add store.go store_test.go
git commit -m "feat: implement JSON file store with atomic operations"
```

### Task 3: Link CRUD Operations

**Files:**
- Modify: `store.go` (add CRUD methods)
- Modify: `store_test.go` (add tests)

**Interfaces:**
- Consumes: `type Link` from Task 1
- Produces:
  - CRUD method implementations: `Create`, `Get`, `GetAll`, `IncrementVisits`, `Delete`

- [ ] **Step 1: Write test for Create and Get operations**

```go
func TestStoreCreateAndGet(t *testing.T) {
    path := "test_crud.json"
    defer os.Remove(path)

    store, err := NewFileStore(path)
    if err != nil {
        t.Fatalf("Failed to create store: %v", err)
    }
    defer store.(*fileStore).close()

    link := Link{
        Code:      "abc123",
        URL:       "https://example.com",
        CreatedAt: time.Now().UTC(),
        Visits:    0,
    }

    err = store.Create(link)
    if err != nil {
        t.Fatalf("Failed to create link: %v", err)
    }

    got, exists := store.Get("abc123")
    if !exists {
        t.Fatal("Link should exist")
    }
    if got.URL != link.URL {
        t.Fatalf("Got URL %q, want %q", got.URL, link.URL)
    }
}
```

- [ ] **Step 2: Implement Create and Get methods**

```go
func (fs *fileStore) Create(link Link) error {
    fs.mu.Lock()
    defer fs.mu.Unlock()

    if _, exists := fs.data[link.Code]; exists {
        return fmt.Errorf("code already exists")
    }
    fs.data[link.Code] = link
    return fs.save()
}

func (fs *fileStore) Get(code string) (Link, bool) {
    fs.mu.RLock()
    defer fs.mu.RUnlock()

    link, exists := fs.data[code]
    return link, exists
}
```

- [ ] **Step 3: Write test for GetAll and Delete**

```go
func TestStoreGetAllAndDelete(t *testing.T) {
    // Test ordering (oldest first) and delete functionality
}
```

- [ ] **Step 4: Implement GetAll, IncrementVisits, and Delete methods**

```go
func (fs *fileStore) GetAll() []Link {
    // Return sorted by CreatedAt ascending
}

func (fs *fileStore) IncrementVisits(code string) error {
    // Atomic increment with mutex
}

func (fs *fileStore) Delete(code string) error {
    fs.mu.Lock()
    defer fs.mu.Unlock()

    if _, exists := fs.data[code]; !exists {
        return fmt.Errorf("code not found")
    }
    delete(fs.data, code)
    return fs.save()
}
```

- [ ] **Step 5: Run all store tests**

Run: `go test ./... -v`
Expect: All tests pass

- [ ] **Step 6: Commit**

```bash
git add store.go store_test.go
git commit -m "feat: implement complete CRUD operations"
```

### Task 4: HTTP Handlers and Routing

**Files:**
- Create: `handlers.go`
- Create: `handlers_test.go`
- Modify: `main.go` (add routing)

**Interfaces:**
- Consumes: `Store` interface from Task 2
- Produces:
  - `func createLinkHandler(store Store) http.HandlerFunc`
  - `func redirectHandler(store Store) http.HandlerFunc`
  - `func listLinksHandler(store Store) http.HandlerFunc`
  - `func deleteLinkHandler(store Store) http.HandlerFunc`

- [ ] **Step 1: Write test for create link handler**

```go
func TestCreateLinkHandler(t *testing.T) {
    // Test with valid request, invalid URL, duplicate alias, etc.
}
```

- [ ] **Step 2: Implement createLinkHandler in handlers.go**

```go
func createLinkHandler(store Store) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Parse request body (max 1MB)
        // Validate URL format (http/https, host, max 2048 chars)
        // Validate alias (regex, not "api")
        // Generate code if no alias
        // Check for duplicate
        // Create link with visits=0 and current time
        // Respond with 201 and JSON
    }
}
```

- [ ] **Step 3: Write test for redirect handler**

```go
func TestRedirectHandler(t *testing.T) {
    // Test success redirect (302), unknown code (404), visit count increment
}
```

- [ ] **Step 4: Implement redirectHandler**

```go
func redirectHandler(store Store) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Extract code from URL
        // Get link from store
        // If not found, 404
        // Increment visits
        // Set Location header and 302
    }
}
```

- [ ] **Step 5: Add admin auth middleware**

```go
func adminAuth(token string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            auth := r.Header.Get("Authorization")
            if !strings.HasPrefix(auth, "Bearer ") {
                http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
                return
            }
            if auth[7:] != token {
                http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

- [ ] **Step 6: Setup routing in main.go**

```go
func main() {
    // ... existing code ...
    store, err := NewFileStore(*dataPath)
    if err != nil {
        log.Fatal(err)
    }
    
    mux := http.NewServeMux()
    mux.HandleFunc("POST /api/links", createLinkHandler(store))
    mux.HandleFunc("GET /api/links", adminAuth(os.Getenv("ADMIN_TOKEN"))(listLinksHandler(store)))
    mux.HandleFunc("DELETE /api/links/{code}", adminAuth(os.Getenv("ADMIN_TOKEN"))(deleteLinkHandler(store)))
    mux.HandleFunc("GET /{code}", redirectHandler(store))
    
    log.Fatal(http.ListenAndServe(*addr, mux))
}
```

- [ ] **Step 7: Run all tests**

Run: `go test ./... -v`
Expect: All tests pass

- [ ] **Step 8: Commit**

```bash
git add handlers.go handlers_test.go main.go
git commit -m "feat: implement HTTP handlers and routing"
```

### Task 5: Code Generation and Validation

**Files:**
- Create: `codegen.go`
- Create: `codegen_test.go`
- Modify: `handlers.go` (use code generation)
- Modify: `handlers_test.go` (add tests)

**Interfaces:**
f- Consumes: Store interface for uniqueness check
- Produces:
  - `func generateCode(store Store) (string, error)`: generates unique 7-char code
  - `func validateURL(url string) error`
  - `func validateAlias(alias string) error`

- [ ] **Step 1: Write test for code generation**

```go
func TestGenerateCode(t *testing.T) {
    // Generate 1000 codes, verify length, charset, uniqueness
}
```

- [ ] **Step 2: Implement generateCode with random [A-Za-z0-9] characters**

```go
func generateCode(store Store) (string, error) {
    const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
    const codeLength = 7
    
    for i := 0; i < 100; i++ {
        b := make([]byte, codeLength)
        for j := range b {
            b[j] = charset[rand.Intn(len(charset))]
        }
        code := string(b)
        
        if _, exists := store.Get(code); !exists {
            return code, nil
        }
    }
    return "", fmt.Errorf("failed to generate unique code")
}
```

- [ ] **Step 3: Write test for URL and alias validation**

```go
func TestValidateURL(t *testing.T) {
    // Test http/https URLs, invalid schemes, missing host, length > 2048
}

func TestValidateAlias(t *testing.T) {
    // Test regex ^[A-Za-z0-9_-]{3,32}$, "api" disallowed
}
```

- [ ] **Step 4: Implement URL and alias validation**

```go
func validateURL(url string) error {
    // Check length
    // Parse with url.Parse
    // Check scheme is http or https
    // Check host is not empty
}

func validateAlias(alias string) error {
    // Check regex
    // Check not "api"
}
```

- [ ] **Step 5: Modify createLinkHandler to use validators and code generator**

- [ ] **Step 6: Run all tests**

Run: `go test ./... -v`
Expect: All tests pass

- [ ] **Step 7: Commit**

```bash
git add codegen.go codegen_test.go handlers.go
git commit -m "feat: implement code generation and request validation"
```

### Task 6: Concurrency and Race Condition Tests

**Files:**
- Create: `concurrency_test.go`
- Modify: `store.go` (ensure thread safety)

**Interfaces:**
- Consumes: Store interface
- Produces: Race condition tests

- [ ] **Step 1: Write concurrent test for visit counting**

```go
func TestConcurrentVisits(t *testing.T) {
    // Create store with single link
    // Launch 100 goroutines that increment visits
    // Wait for all goroutines
    // Verify final visit count is 100
}
```

- [ ] **Step 2: Write concurrent test for create operations**

```go
func TestConcurrentCreate(t *testing.T) {
    // Test creating links with same alias/code concurrently
    // Should result in only one success (409 for others)
}
```

- [ ] **Step 3: Run tests with race detector**

Run: `go test -race ./...`
Expect: No data races detected

- [ ] **Step 4: Add atomic file write simulation test**

```go
func TestCrashRecovery(t *testing.T) {
    // Simulate crash during save by killing process
    // Verify store can reload from partial file
}
```

- [ ] **Step 5: Run all tests including race detector**

Run: `go test -race ./... -v`
Expect: All tests pass, no races

- [ ] **Step 6: Commit**

```bash
git add concurrency_test.go store.go
git commit -m "feat: add concurrency tests and ensure thread safety"
```

### Task 7: Integration and Final Testing

**Files:**
- Create: `integration_test.go`
- Modify: `main.go` (final cleanup)

**Interfaces:**
- Consumes: Complete application
- Produces: End-to-end integration tests

- [ ] **Step 1: Write integration test that spins up server**

```go
func TestIntegration(t *testing.T) {
    // Start server in goroutine
    // Test full flow: create link, redirect, list, delete
    // Test admin auth required for list/delete
}
```

- [ ] **Step 2: Test error responses and status codes**

```go
func TestErrorResponses(t *testing.T) {
    // Test 400, 401, 404, 405, 409, 413
    // Verify JSON error format
}
```

- [ ] **Step 3: Test command-line arguments**

```go
func TestMainArgs(t *testing.T) {
    // Test -addr and -data flags
    // Test ADMIN_TOKEN requirement
}
```

卸载 ] **Step 4: Run full test suite**

Run: `go test ./... -race -v`
Run: `go vet ./...`
Run: `go build -o shortener .`
Expect: All pass, build successful

- [ ] **Step 5: Create example test data and README**

```go
// Example usage in README or example test
```

- [ ] **Step 6: Final commit**

```bash
git add integration_test.go main.go README.md
git commit -m "feat: complete integration testing and documentation"
```