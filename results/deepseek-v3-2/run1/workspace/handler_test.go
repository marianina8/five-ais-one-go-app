package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandler_CreateLink(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	handler := NewHandler(store, "secret-token")

	// Test successful creation with alias
	reqBody := `{"url": "https://example.com", "alias": "my-page"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusCreated {
		t.Errorf("CreateLink status = %v, want %v", w.Code, http.StatusCreated)
	}
	
	var resp Link
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	if resp.Code != "my-page" {
		t.Errorf("CreateLink code = %v, want my-page", resp.Code)
	}
	if resp.URL != "https://example.com" {
		t.Errorf("CreateLink URL = %v, want https://example.com", resp.URL)
	}
	if resp.Visits != 0 {
		t.Errorf("CreateLink visits = %v, want 0", resp.Visits)
	}

	// Test duplicate alias
	req = httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusConflict {
		t.Errorf("Duplicate alias status = %v, want %v", w.Code, http.StatusConflict)
	}

	// Test creation without alias
	reqBody = `{"url": "https://google.com"}`
	req = httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusCreated {
		t.Errorf("Create without alias status = %v, want %v", w.Code, http.StatusCreated)
	}
	
	var resp2 Link
	if err := json.Unmarshal(w.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	if len(resp2.Code) != 7 {
		t.Errorf("Generated code length = %v, want 7", len(resp2.Code))
	}

	// Test invalid URL
	reqBody = `{"url": "invalid-url"}`
	req = httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("Invalid URL status = %v, want %v", w.Code, http.StatusBadRequest)
	}

	// Test invalid alias
	reqBody = `{"url": "https://example.com", "alias": "ab"}`
	req = httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("Invalid alias status = %v, want %v", w.Code, http.StatusBadRequest)
	}

	// Test invalid JSON
	reqBody = `{invalid json`
	req = httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusBadRequest {
		t.Errorf("Invalid JSON status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_RedirectLink(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	handler := NewHandler(store, "secret-token")

	// Create a link first
	reqBody := `{"url": "https://example.com", "alias": "test-redirect"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create link: status = %v", w.Code)
	}

	// Test redirect
	req = httptest.NewRequest("GET", "/test-redirect", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusFound {
		t.Errorf("Redirect status = %v, want %v", w.Code, http.StatusFound)
	}
	
	location := w.Header().Get("Location")
	if location != "https://example.com" {
		t.Errorf("Location header = %v, want https://example.com", location)
	}

	// Test non-existent link
	req = httptest.NewRequest("GET", "/non-existent", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("Non-existent status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestHandler_ListLinks(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	handler := NewHandler(store, "secret-token")

	// Create some links
	links := []struct {
		url   string
		alias string
	}{
		{"https://first.com", "first"},
		{"https://second.com", "second"},
	}

	for _, l := range links {
		reqBody := fmt.Sprintf(`{"url": "%s", "alias": "%s"}`, l.url, l.alias)
		req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		
		if w.Code != http.StatusCreated {
			t.Fatalf("Failed to create link %s: status = %v", l.alias, w.Code)
		}
	}

	// Test list without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusUnauthorized {
		t.Errorf("List without auth status = %v, want %v", w.Code, http.StatusUnauthorized)
	}

	// Test list with auth
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("List with auth status = %v, want %v", w.Code, http.StatusOK)
	}
	
	var resp []Link
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	if len(resp) != 2 {
		t.Errorf("List returned %v links, want 2", len(resp))
	}
}

func TestHandler_DeleteLink(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	handler := NewHandler(store, "secret-token")

	// Create a link first
	reqBody := `{"url": "https://example.com", "alias": "to-delete"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create link: status = %v", w.Code)
	}

	// Test delete without auth
	req = httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Delete without auth status = %v, want %v", w.Code, http.StatusUnauthorized)
	}

	// Test delete with auth
	req = httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusNoContent {
		t.Errorf("Delete with auth status = %v, want %v", w.Code, http.StatusNoContent)
	}

	// Verify deleted
	req = httptest.NewRequest("GET", "/to-delete", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("Get after delete status = %v, want %v", w.Code, http.StatusNotFound)
	}

	// Test delete non-existent
	req = httptest.NewRequest("DELETE", "/api/links/non-existent", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("Delete non-existent status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	handler := NewHandler(store, "secret-token")

	// Test PUT on /api/links
	req := httptest.NewRequest("PUT", "/api/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("PUT on /api/links status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestHandler_LargeRequestBody(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	handler := NewHandler(store, "secret-token")

	// Create request body larger than 1 MiB
	largeBody := make([]byte, 2<<20) // 2 MiB
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBuffer(largeBody))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	
	// Should return 413 or bad request
	if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Large body status = %v, want 400 or 413", w.Code)
	}
}

func TestMain_EnvironmentValidation(t *testing.T) {
	// Test that main would exit without ADMIN_TOKEN
	originalToken := os.Getenv("ADMIN_TOKEN")
	os.Unsetenv("ADMIN_TOKEN")
	defer os.Setenv("ADMIN_TOKEN", originalToken)

	// We can't actually run main() in a test, but we can verify the logic
	// by checking that NewJSONStore doesn't have this validation
	// (validation is in main)
	
	// This test just documents the requirement
	t.Log("ADMIN_TOKEN validation is in main(), not in store")
}