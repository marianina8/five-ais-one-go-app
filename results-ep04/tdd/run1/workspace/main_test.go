package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	// Clean up test data files
	cleanupTestFiles()
	os.Exit(code)
}

func cleanupTestFiles() {
	files, _ := filepath.Glob("test_data_*.json")
	for _, f := range files {
		os.Remove(f)
	}
}

func TestServer_CreateLink(t *testing.T) {
	tempFile := "test_data_create.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Test valid link creation without alias
	reqBody := `{"url": "https://example.com"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateLink(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var response Link
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", response.URL)
	}
	if len(response.Code) != 7 {
		t.Errorf("expected code length 7, got %d", len(response.Code))
	}
	if response.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", response.Visits)
	}

	// Test valid link creation with alias
	reqBody = `{"url": "https://example.org", "alias": "test-alias"}`
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handleCreateLink(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.Code != "test-alias" {
		t.Errorf("expected code test-alias, got %s", response.Code)
	}

	// Test duplicate alias conflict
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handleCreateLink(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409 for duplicate alias, got %d", w.Code)
	}
}

func TestServer_CreateLinkValidation(t *testing.T) {
	tempFile := "test_data_validation.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	testCases := []struct {
		name     string
		reqBody  string
		wantCode int
	}{
		{
			name:     "invalid url scheme",
			reqBody:  `{"url": "ftp://example.com"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing host",
			reqBody:  `{"url": "http://"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid alias too short",
			reqBody:  `{"url": "https://example.com", "alias": "ab"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid alias characters",
			reqBody:  `{"url": "https://example.com", "alias": "test@alias"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "reserved alias api",
			reqBody:  `{"url": "https://example.com", "alias": "api"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "malformed json",
			reqBody:  `{"url": "https://example.com"`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty request body",
			reqBody:  "",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/links", strings.NewReader(tc.reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleCreateLink(w, req)

			if w.Code != tc.wantCode {
				t.Errorf("expected status %d, got %d", tc.wantCode, w.Code)
			}
		})
	}
}

func TestServer_HandleRedirect(t *testing.T) {
	tempFile := "test_data_redirect.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Create a link first
	_, err = store.CreateLink("https://example.com", "test-redirect")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Test redirect
	req := httptest.NewRequest("GET", "/test-redirect", nil)
	w := httptest.NewRecorder()

	server.handleRedirect(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if location != "https://example.com" {
		t.Errorf("expected Location https://example.com, got %s", location)
	}

	// Check that visits were incremented
	updatedLink, err := store.IncrementVisits("test-redirect")
	if err != nil {
		t.Fatalf("failed to get updated link: %v", err)
	}

	if updatedLink.Visits != 2 { // 1 from redirect + 1 from IncrementVisits call
		t.Errorf("expected 2 visits, got %d", updatedLink.Visits)
	}

	// Test non-existent code
	req = httptest.NewRequest("GET", "/non-existent", nil)
	w = httptest.NewRecorder()

	server.handleRedirect(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent code, got %d", w.Code)
	}
}

func TestServer_ListLinks(t *testing.T) {
	tempFile := "test_data_list.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Create some links
	link1, err := store.CreateLink("https://example1.com", "link1")
	if err != nil {
		t.Fatalf("failed to create link1: %v", err)
	}

	// Wait a bit to ensure different timestamps
	time.Sleep(1 * time.Millisecond)

	link2, err := store.CreateLink("https://example2.com", "link2")
	if err != nil {
		t.Fatalf("failed to create link2: %v", err)
	}

	// Test without admin token (should fail)
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()

	server.handleListLinks(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without admin token, got %d", w.Code)
	}

	// Test with admin token
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleListLinks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var links []Link
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatalf("failed to unmarshal links: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}

	// Check ordering (oldest first)
	if links[0].Code != link1.Code {
		t.Errorf("expected first link to be %s (older), got %s", link1.Code, links[0].Code)
	}
	if links[1].Code != link2.Code {
		t.Errorf("expected second link to be %s (newer), got %s", link2.Code, links[1].Code)
	}
}

func TestServer_DeleteLink(t *testing.T) {
	tempFile := "test_data_delete.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Create a link
	_, err = store.CreateLink("https://example.com", "to-delete")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Test delete without admin token (should fail)
	req := httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	w := httptest.NewRecorder()

	server.handleDeleteLink(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without admin token, got %d", w.Code)
	}

	// Test delete with admin token
	req = httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleDeleteLink(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	// Verify link is gone
	req = httptest.NewRequest("GET", "/to-delete", nil)
	w = httptest.NewRecorder()

	server.handleRedirect(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 after delete, got %d", w.Code)
	}

	// Test delete non-existent link
	req = httptest.NewRequest("DELETE", "/api/links/non-existent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleDeleteLink(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent link, got %d", w.Code)
	}
}

func TestServer_ServeHTTP(t *testing.T) {
	tempFile := "test_data_serve.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Test redirect via ServeHTTP
	_, err = store.CreateLink("https://serve.example.com", "serve-test")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	req := httptest.NewRequest("GET", "/serve-test", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}

	// Test API routes via ServeHTTP
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(`{"url": "https://api.example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	// Test method not allowed
	req = httptest.NewRequest("PUT", "/api/links", nil)
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}

	// Test not found
	req = httptest.NewRequest("GET", "/unknown/path", nil)
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestFileStore_Concurrency(t *testing.T) {
	tempFile := "test_data_concurrency.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create initial link
	_, err = store.CreateLink("https://concurrent.example.com", "concurrent")
	if err != nil {
		t.Fatalf("failed to create initial link: %v", err)
	}

	// Run multiple goroutines to increment visits concurrently
	const goroutines = 10
	const increments = 100
	done := make(chan bool, goroutines)
	errors := make(chan error, goroutines*increments)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < increments; j++ {
				_, err := store.IncrementVisits("concurrent")
				if err != nil {
					errors <- fmt.Errorf("goroutine %d, increment %d: %v", id, j, err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines; i++ {
		<-done
	}

	close(errors)
	close(done)

	// Check for errors
	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}

	// Verify total visits
	link, err := store.IncrementVisits("concurrent") // One more to get the value
	if err != nil {
		t.Fatalf("failed to get final link: %v", err)
	}

	expectedVisits := goroutines*increments + 1 // +1 for this call
	if link.Visits != expectedVisits {
		t.Errorf("expected %d visits, got %d", expectedVisits, link.Visits)
	}
}

func TestFileStore_Persistence(t *testing.T) {
	tempFile := "test_data_persistence.json"
	defer os.Remove(tempFile)

	// Create store and add a link
	store1, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store1: %v", err)
	}

	link, err := store1.CreateLink("https://persist.example.com", "persist")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	store1.Close()

	// Create new store from same file
	store2, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store2: %v", err)
	}
	defer store2.Close()

	// Check that link exists
	retrieved, err := store2.IncrementVisits("persist")
	if err != nil {
		t.Fatalf("failed to retrieve persisted link: %v", err)
	}

	if retrieved.URL != link.URL {
		t.Errorf("persisted URL mismatch: got %s, want %s", retrieved.URL, link.URL)
	}
	if retrieved.Code != link.Code {
		t.Errorf("persisted Code mismatch: got %s, want %s", retrieved.Code, link.Code)
	}
}

func TestLargeRequestBody(t *testing.T) {
	tempFile := "test_data_large.json"
	defer os.Remove(tempFile)

	store, err := NewFileStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Create a request body larger than 1 MiB
	largeBody := make([]byte, 2<<20) // 2 MiB
	for i := range largeBody {
		largeBody[i] = 'a'
	}
	// Make it valid JSON
	copy(largeBody, `{"url": "https://example.com"}`)

	req := httptest.NewRequest("POST", "/api/links", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateLink(w, req)

	// Should reject with 413 Payload Too Large
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for large request body, got %d", w.Code)
	}
}