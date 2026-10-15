# URL Shortener Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a concurrent URL shortener service with JSON file persistence, admin authentication, and REST API.

**Architecture:** HTTP server with in-memory sync.Map for fast concurrent access, backed by atomic JSON file writes. Admin auth via Bearer token, link storage with unique code generation, visit tracking, and file persistence.

**Tech Stack:** Go 1.24, standard library only (net/http, sync, encoding/json, etc.)

**Spec:** Provided in the conversation - Build a URL shortener in Go

## Global Constraints

- Go 1.24, standard library only (no third-party modules; the network is off)
- Module name `shortener`
- Program builds with `go build -o shortener .`
- Tests run with `go test ./...`
- Default address `:8080`, default data file `data.json`
- ADMIN_TOKEN required at startup (non-empty, exits if empty)
- JSON response format for errors: `{"error": "<message>"}`
- Admin auth: `Authorization: Bearer <ADMIN_TOKEN>`
- URL validation: absolute http/https URLs, max 2048 chars
- Alias validation: `^[A-Za-z0-9_-]{3,32}$`, not `api`
- Random code generation: 7 characters from `[A-Za-z0-9]`
- Request body limit: 1 MiB
- Concurrent safety with sync.Map + atomic file writes
- Atomic persistence: every change saved before response

## Review Focus

1. Concurrent visit counting should not lose increments under high load
2. File writes must be atomic to prevent corruption on crash
3. Random code generation must guarantee uniqueness
4. Admin auth must reject missing/invalid tokens for protected endpoints
5. Redirects must properly increment visit counts before response

---

## Task Structure

### Task 1: Project Setup and Core Data Model

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `internal/model/link.go`
- Create: `internal/store/store.go`
- Create: `internal/store/json_store.go`

**Interfaces:**
- Consumes: Nothing (first task)
- Produces: 
  - `model.Link` struct with fields: `URL`, `Code`, `Alias`, `CreatedAt`, `Visits`
  - `store.Store` interface with methods: `Create`, `Get`, `IncrementVisits`, `Delete`, `List`, `Save`, `Load`
  - File path for persistence

- [ ] **Step 1: Create go.mod with module name "shortener"**

```go
module shortener

go 1.24
```

- [ ] **Step 2: Write Link model in internal/model/link.go**

```go
package model

import "time"

type Link struct {
	URL       string    `json:"url"`
	Code      string    `json:"code"`
	Alias     string    `json:"alias,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}
```

- [ ] **Step 3: Write Store interface in internal/store/store.go**

```go
package store

import "shortener/internal/model"

type Store interface {
	Create(url, alias string) (*model.Link, error)
	Get(code string) (*model.Link, error)
	IncrementVisits(code string) error
	Delete(code string) error
	List() []*model.Link
	Save() error
	Load() error
}
```

- [ ] **Step 4: Create main.go skeleton with flag parsing**

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "server address")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	fmt.Printf("Starting server on %s with data file %s\n", *addr, *dataFile)
}
```

### Task 2: JSON File Store Implementation

**Files:**
- Modify: `internal/store/json_store.go`
- Test: `internal/store/json_store_test.go`

**Interfaces:**
- Consumes: `model.Link` from Task 1, `store.Store` interface
- Produces: `jsonStore` struct implementing `Store` with atomic file writes

- [ ] **Step 1: Write tests for json_store basic operations**

```go
func TestJsonStore_CreateGet(t *testing.T) {
	s := NewJsonStore(t.TempDir() + "/data.json")
	defer os.Remove(t.TempDir() + "/data.json")
	
	link, err := s.Create("https://example.com", "")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", link.URL)
	assert.Len(t, link.Code, 7)
	
	link2, err := s.Get(link.Code)
	require.NoError(t, err)
	assert.Equal(t, link.URL, link2.URL)
}
```

- [ ] **Step 2: Implement jsonStore struct with sync.Map and file locks**

```go
type jsonStore struct {
	mu    sync.RWMutex
	links map[string]*model.Link
	path  string
}

func NewJsonStore(path string) *jsonStore {
	return &jsonStore{
		links: make(map[string]*model.Link),
		path:  path,
	}
}
```

- [ ] **Step 3: Implement Create with random code generation (7 chars [A-Za-z0-9])**

```go
func (s *jsonStore) Create(url, alias string) (*model.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check alias if provided
	if alias != "" {
		if _, exists := s.links[alias]; exists {
			return nil, fmt.Errorf("alias already taken")
		}
	}
	
	code := alias
	if code == "" {
		code = generateRandomCode(7)
		for _, exists := s.links[code]; exists; {
			code = generateRandomCode(7)
		}
	}
	
	link := &model.Link{
		URL:       url,
		Code:      code,
		Alias:     alias,
		CreatedAt: time.Now(),
		Visits:    0,
	}
	
	s.links[code] = link
	return link, s.Save()
}
```

- [ ] **Step 4: Implement atomic file Save() using temporary file and rename**

```go
func (s *jsonStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	tmpFile := s.path + ".tmp"
	data, err := json.MarshalIndent(s.links, "", "  ")
	if err != nil {
		return err
	}
	
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tmpFile, s.path)
}
```

### Task 3: HTTP Handler Structure and Validation

**Files:**
- Create: `internal/handler/handler.go`
- Create: `internal/handler/create.go`
- Create: `internal/handler/redirect.go`
- Test: `internal/handler/handler_test.go`

**Interfaces:**
- Consumes: `store.Store` from Task 2
- Produces: HTTP handlers with validation middleware, admin auth middleware

- [ ] **Step 1: Write tests for URL validation**

```go
func TestValidateURL(t *testing.T) {
	tests := []struct {
		url    string
		valid  bool
	}{
		{"https://example.com", true},
		{"http://localhost:8080", true},
		{"ftp://example.com", false},
		{"example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		got := validateURL(tt.url)
		assert.Equal(t, tt.valid, got, "validateURL(%q)", tt.url)
	}
}
```

- [ ] **Step 2: Write tests for alias validation**

```go
func TestValidateAlias(t *testing.T) {
	tests := []struct {
		alias  string
		valid  bool
	}{
		{"my-alias_123", true},
		{"api", false},
		{"ab", false},
		{"too-long-alias-that-is-more-than-32-chars", false},
		{"", true}, // empty is valid (means generate random)
		{"invalid!char", false},
	}
	for _, tt := range tests {
		got := validateAlias(tt.alias)
		assert.Equal(t, tt.valid, got, "validateAlias(%q)", tt.alias)
	}
}
```

- [ ] **Step 3: Implement validation functions**

```go
func validateURL(url string) bool {
	parsed, err := url.Parse(url)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https" &&
		parsed.Host != "" &&
		len(url) <= 2048
}

func validateAlias(alias string) bool {
	if alias == "" {
		return true
	}
	if alias == "api" {
		return false
	}
	if len(alias) < 3 || len(alias) > 32 {
		return false
	}
	matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, alias)
	return matched
}
```

- [ ] **Step 4: Create admin auth middleware**

```go
func adminAuth(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid authorization")
			return
		}
		
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != adminToken {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		
		next.ServeHTTP(w, r)
	})
}
```

### Task 4: Create Link Endpoint

**Files:**
- Modify: `internal/handler/create.go`
- Test: `internal/handler/create_test.go`

**Interfaces:**
- Consumes: `store.Store`, validation functions from Task 3
- Produces: `POST /api/links` handler with JSON request/response

- [ ] **Step 1: Write test for create link endpoint**

```go
func TestCreateLink(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	h := NewHandler(s, "test-token")
	
	req := CreateLinkRequest{
		URL:   "https://example.com",
		Alias: "test-alias",
	}
	jsonReq, _ := json.Marshal(req)
	
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/links", bytes.NewReader(jsonReq))
	
	h.CreateLink(rr, r)
	assert.Equal(t, http.StatusCreated, rr.Code)
	
	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	assert.Equal(t, "test-alias", resp["code"])
	assert.Equal(t, "https://example.com", resp["url"])
}
```

- [ ] **Step 2: Implement CreateLink handler with request struct**

```go
type CreateLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

type CreateLinkResponse struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

func (h *Handler) CreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	// Limit request body to 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	
	var req CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	
	// Validate URL
	if !validateURL(req.URL) {
		writeError(w, http.StatusBadRequest, "invalid URL")
		return
	}
	
	// Validate alias if provided
	if req.Alias != "" && !validateAlias(req.Alias) {
		writeError(w, http.StatusBadRequest, "invalid alias")
		return
	}
	
	link, err := h.store.Create(req.URL, req.Alias)
	if err != nil {
		if strings.Contains(err.Error(), "already taken") {
			writeError(w, http.StatusConflict, "alias already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateLinkResponse{
		Code:      link.Code,
		URL:       link.URL,
		CreatedAt: link.CreatedAt,
		Visits:    link.Visits,
	})
}
```

### Task 5: Redirect and List Endpoints

**Files:**
- Modify: `internal/handler/redirect.go`
- Create: `internal/handler/list.go`
- Test: `internal/handler/redirect_test.go`
- Test: `internal/handler/list_test.go`

**Interfaces:**
- Consumes: `store.Store` from Task 2
- Produces: `GET /{code}` redirect handler and `GET /api/links` list handler

- [ ] **Step 1: Write test for redirect endpoint**

```go
func TestRedirectHandler(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	h := NewHandler(s, "test-token")
	
	// First create a link
	link, _ := s.Create("https://example.com", "test-code")
	
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test-code", nil)
	
	h.Redirect(rr, r)
	
	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "https://example.com", rr.Header().Get("Location"))
	
	// Verify visit count increased
	stored, _ := s.Get("test-code")
	assert.Equal(t, int64(1), stored.Visits)
}
```

- [ ] **Step 2: Implement Redirect handler with concurrent visit counting**

```go
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	
	link, err := h.store.Get(code)
	if err != nil {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	
	// Increment visits before redirect
	if err := h.store.IncrementVisits(code); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	
	http.Redirect(w, r, link.URL, http.StatusFound)
}
```

- [ ] **Step 3: Write test for list endpoint with admin auth**

```go
func TestListLinks(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	h := NewHandler(s, "test-token")
	
	s.Create("https://example.com/1", "code1")
	s.Create("https://example.com/2", "code2")
	
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/links", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	
	h.ListLinks(rr, r)
	
	assert.Equal(t, http.StatusOK, rr.Code)
	
	var links []model.Link
	json.Unmarshal(rr.Body.Bytes(), &links)
	assert.Len(t, links, 2)
}
```

- [ ] **Step 4: Implement ListLinks handler with admin auth**

```go
func (h *Handler) ListLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	links := h.store.List()
	// Sort by CreatedAt ascending
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}
```

### Task 6: Delete Endpoint and Router Setup

**Files:**
- Create: `internal/handler/delete.go`
- Modify: `internal/handler/handler.go` (add routing)
- Test: `internal/handler/delete_test.go`

**Interfaces:**
- Consumes: `store.Store` from Task 2, admin auth from Task 3
- Produces: `DELETE /api/links/{code}` handler and complete router

- [ ] **Step 1: Write test for delete endpoint**

```go
func TestDeleteLink(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	h := NewHandler(s, "test-token")
	
	s.Create("https://example.com", "test-code")
	
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("DELETE", "/api/links/test-code", nil)
	r.Header.Set("Authorization", "Bearer test-token")
	
	h.DeleteLink(rr, r)
	
	assert.Equal(t, http.StatusNoContent, rr.Code)
	
	// Verify link is deleted
	_, err := s.Get("test-code")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Implement DeleteLink handler**

```go
func (h *Handler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	if code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}
	
	err := h.store.Delete(code)
	if err != nil {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Implement router setup in handler.go**

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	
	switch {
	case path == "/api/links" && r.Method == "POST":
		h.CreateLink(w, r)
	case path == "/api/links" && r.Method == "GET":
		h.adminMiddleware(http.HandlerFunc(h.ListLinks)).ServeHTTP(w, r)
	case strings.HasPrefix(path, "/api/links/"):
		h.adminMiddleware(http.HandlerFunc(h.DeleteLink)).ServeHTTP(w, r)
	default:
		// Check if it's a redirect
		if r.Method == "GET" {
			h.Redirect(w, r)
		} else {
			writeError(w, http.StatusNotFound, "not found")
		}
	}
}
```

### Task 7: Main Server Integration and Concurrency Testing

**Files:**
- Modify: `main.go`
- Create: `cmd/server/server_test.go` (integration tests)
- Test: Concurrency tests

**Interfaces:**
- Consumes: All previous components
- Produces: Complete server with concurrent safety testing

- [ ] **Step 1: Update main.go to integrate all components**

```go
func main() {
	addr := flag.String("addr", ":8080", "server address")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	store := jsonstore.NewJsonStore(*dataFile)
	if err := store.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to load data file: %v", err)
	}

	handler := handler.NewHandler(store, adminToken)
	
	log.Printf("Starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Write concurrent test for visit counting**

```go
func TestConcurrentVisits(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	link, _ := s.Create("https://example.com", "test-code")
	
	var wg sync.WaitGroup
	visits := 100
	
	for i := 0; i < visits; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.IncrementVisits("test-code")
		}()
	}
	
	wg.Wait()
	
	stored, _ := s.Get("test-code")
	assert.Equal(t, int64(visits), stored.Visits)
}
```

- [ ] **Step 3: Write concurrent test for link creation uniqueness**

```go
func TestConcurrentCreateUniqueCodes(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	
	codes := sync.Map{}
	var wg sync.WaitGroup
	creations := 100
	
	for i := 0; i < creations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			link, err := s.Create("https://example.com", "")
			require.NoError(t, err)
			// Ensure code is unique
			_, loaded := codes.LoadOrStore(link.Code, struct{}{})
			assert.False(t, loaded, "duplicate code generated")
		}()
	}
	
	wg.Wait()
	
	links := s.List()
	assert.Len(t, links, creations)
}
```

## Self-Review

**1. Spec coverage:** All spec requirements covered:
   - POST /api/links with validation ✓
   - GET /{code} with redirect and visit counting ✓  
   - GET /api/links with admin auth ✓
   - DELETE /api/links/{code} with admin auth ✓
   - ADMIN_TOKEN required at startup ✓
   - JSON file persistence with atomic writes ✓
   - Concurrent safety with sync.Map and mutexes ✓
   - Request body limit 1 MiB ✓
   - URL validation (http/https, max 2048) ✓
   - Alias validation (regex, length, not "api") ✓

**2. Step scan:** All steps specify exact file paths, function signatures, and test assertions. No vague steps.

**3. Type consistency:** Function and type names consistent across tasks (e.g., `Create`, `Get`, `IncrementVisits`).

**4. Review Focus:** Covered review focus items:
   - Concurrent visit counting tested in Task 7 ✓
   - Atomic file writes implemented in Task 2 ✓
   - Unique code generation tested in Task 7 ✓
   - Admin auth tested in Tasks 5 and 6 ✓
   - Redirect visit increment tested in Task 5 ✓

**5. Proportion:** Plan is comprehensive but not overly verbose - focuses on interfaces and test coverage rather than transcribing implementation code.