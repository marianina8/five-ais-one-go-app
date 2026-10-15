package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempFilename(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test-data.json")
}

func TestCreateLink(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}

	// Test creating link without alias
	link, err := store.Create("https://example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(link.Code) != 7 {
		t.Errorf("expected 7 character code, got %q", link.Code)
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %q", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link.Visits)
	}
	if link.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}

	// Test creating link with alias
	link2, err := store.Create("https://example.org", "my-alias")
	if err != nil {
		t.Fatal(err)
	}
	if link2.Code != "my-alias" {
		t.Errorf("expected code 'my-alias', got %q", link2.Code)
	}

	// Test duplicate alias
	_, err = store.Create("https://example.net", "my-alias")
	if err == nil || err.Error() != "alias already taken" {
		t.Errorf("expected 'alias already taken' error, got %v", err)
	}

	// Test invalid URL
	_, err = store.Create("ftp://example.com", "")
	if err == nil || !strings.Contains(err.Error(), "URL must start with http://") {
		t.Errorf("expected URL validation error, got %v", err)
	}

	// Test invalid alias (too short)
	_, err = store.Create("https://example.com", "ab")
	if err == nil || err.Error() != "alias must match ^[A-Za-z0-9_-]{3,32}$" {
		t.Errorf("expected alias validation error, got %v", err)
	}

	// Test reserved alias
	_, err = store.Create("https://example.com", "api")
	if err == nil || err.Error() != "alias 'api' is reserved" {
		t.Errorf("expected 'api' reserved error, got %v", err)
	}
}

func TestGetAndVisit(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatal(err)
	}

	// Test Get
	retrieved, err := store.Get("test123")
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.Code != "test123" {
		t.Errorf("expected code 'test123', got %q", retrieved.Code)
	}

	// Test Visit increments count
	visited, err := store.Visit("test123")
	if err != nil {
		t.Fatal(err)
	}
	if visited.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", visited.Visits)
	}

	// Verify Get sees updated count
	retrieved, err = store.Get("test123")
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.Visits != 1 {
		t.Errorf("expected 1 visit after increment, got %d", retrieved.Visits)
	}

	// Test non-existent code
	_, err = store.Get("nonexistent")
	if err == nil || err.Error() != "not found" {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestList(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}

	// Create links in reverse order to test sorting
	store.Create("https://example.com/2", "link2")
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	store.Create("https://example.com/1", "link1")

	links, err := store.List()
	if err != nil {
		t.Fatal(err)
	}

	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}

	// Should be sorted oldest first
	if links[0].Code != "link2" {
		t.Errorf("expected first link to be 'link2', got %q", links[0].Code)
	}
	if links[1].Code != "link1" {
		t.Errorf("expected second link to be 'link1', got %q", links[1].Code)
	}
}

func TestDelete(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}

	store.Create("https://example.com", "test123")

	// Verify it exists
	_, err = store.Get("test123")
	if err != nil {
		t.Fatal(err)
	}

	// Delete it
	err = store.Delete("test123")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's gone
	_, err = store.Get("test123")
	if err == nil || err.Error() != "not found" {
		t.Errorf("expected 'not found' error, got %v", err)
	}

	// Test deleting non-existent
	err = store.Delete("nonexistent")
	if err == nil || err.Error() != "not found" {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestHTTPCreateLink(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store, "test-token")

	// Test successful creation
	reqBody := `{"url": "https://example.com", "alias": "test-alias"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var response Link
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "test-alias" {
		t.Errorf("expected code 'test-alias', got %q", response.Code)
	}
	if response.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %q", response.URL)
	}

	// Test duplicate alias
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", w.Code)
	}

	// Test invalid JSON
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}

	// Test request too large - create a very long URL that makes the JSON > 1 MiB
	var buf bytes.Buffer
	buf.WriteString(`{"url": "`)
	// Create a URL with a very long query string
	for i := 0; i < 1<<20; i++ { // Write 1 MiB of 'a's
		buf.WriteByte('a')
	}
	buf.WriteString(`", "alias": "test"}`)
	
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", &buf)
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPRedirect(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store, "test-token")

	// Create a link first
	store.Create("https://example.com", "redirect-test")

	req := httptest.NewRequest("GET", "/redirect-test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "https://example.com" {
		t.Errorf("expected Location 'https://example.com', got %q", location)
	}

	// Test non-existent link
	req = httptest.NewRequest("GET", "/nonexistent", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHTTPListLinks(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store, "test-token")

	store.Create("https://example.com/1", "link1")
	store.Create("https://example.com/2", "link2")

	// Test without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w.Code)
	}

	// Test with wrong token
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 with wrong token, got %d", w.Code)
	}

	// Test with correct token
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var links []Link
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}
}

func TestHTTPDeleteLink(t *testing.T) {
	store, err := NewJSONStore(tempFilename(t))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store, "test-token")

	store.Create("https://example.com", "delete-test")

	// Test without auth
	req := httptest.NewRequest("DELETE", "/api/links/delete-test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w.Code)
	}

	// Test with correct token
	req = httptest.NewRequest("DELETE", "/api/links/delete-test", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	// Verify link is gone
	req = httptest.NewRequest("GET", "/delete-test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 after delete, got %d", w.Code)
	}

	// Test delete non-existent with auth
	req = httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestPersistence(t *testing.T) {
	tempFile := tempFilename(t)
	
	// Create store and add data
	store1, err := NewJSONStore(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	store1.Create("https://example.com/1", "persist1")
	store1.Create("https://example.com/2", "persist2")
	store1.Visit("persist1") // Increment visits

	// Create new store instance (simulating restart)
	store2, err := NewJSONStore(tempFile)
	if err != nil {
		t.Fatal(err)
	}

	// Verify data persisted
	link, err := store2.Get("persist1")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://example.com/1" {
		t.Errorf("expected URL 'https://example.com/1', got %q", link.URL)
	}
	if link.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", link.Visits)
	}

	link, err = store2.Get("persist2")
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://example.com/2" {
		t.Errorf("expected URL 'https://example.com/2', got %q", link.URL)
	}
}