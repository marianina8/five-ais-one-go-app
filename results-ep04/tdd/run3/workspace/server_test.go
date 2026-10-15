package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestServer_CreateLink(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-server-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Test valid link creation with alias
	reqBody := `{"url": "https://example.com", "alias": "test"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateLink(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var link Link
	if err := json.NewDecoder(w.Body).Decode(&link); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if link.Code != "test" {
		t.Errorf("expected code 'test', got %s", link.Code)
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link.Visits)
	}

	// Test duplicate alias
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	server.handleCreateLink(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409 for duplicate alias, got %d", w.Code)
	}

	// Test valid link creation without alias
	reqBody = `{"url": "https://example2.com"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	server.handleCreateLink(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	// Test invalid URL
	reqBody = `{"url": "not-a-url", "alias": "test2"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	server.handleCreateLink(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid URL, got %d", w.Code)
	}

	// Test invalid alias
	reqBody = `{"url": "https://example.com", "alias": "a"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	server.handleCreateLink(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid alias, got %d", w.Code)
	}

	// Test missing Content-Type
	reqBody = `{"url": "https://example.com", "alias": "test3"}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/links", bytes.NewReader([]byte(reqBody)))
	server.handleCreateLink(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status 415 for missing Content-Type, got %d", w.Code)
	}
}

func TestServer_RedirectLink(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-server-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Create a link first
	_, err = store.Add("https://example.com/redirect", "test-redirect")
	if err != nil {
		t.Fatal(err)
	}

	// Test redirect
	req := httptest.NewRequest("GET", "/test-redirect", nil)
	w := httptest.NewRecorder()

	server.handleRoot(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if location != "https://example.com/redirect" {
		t.Errorf("expected Location header 'https://example.com/redirect', got %s", location)
	}

	// Verify visits were incremented
	link, err := store.Get("test-redirect")
	if err != nil {
		t.Fatal(err)
	}
	if link.Visits != 1 {
		t.Errorf("expected 1 visit after redirect, got %d", link.Visits)
	}

	// Test non-existent link
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/nonexistent", nil)
	server.handleRoot(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestServer_ListLinks(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-server-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Create links
	_, err = store.Add("https://example1.com", "test1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Add("https://example2.com", "test2")
	if err != nil {
		t.Fatal(err)
	}

	// Test without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()

	server.handleListLinks(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w.Code)
	}

	// Test with auth
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleListLinks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var links []Link
	if err := json.NewDecoder(w.Body).Decode(&links); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}

	// Test with invalid token
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()

	server.handleListLinks(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 with wrong token, got %d", w.Code)
	}
}

func TestServer_DeleteLink(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-server-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Create link
	_, err = store.Add("https://example.com", "test-delete")
	if err != nil {
		t.Fatal(err)
	}

	// Test without auth
	req := httptest.NewRequest("DELETE", "/api/links/test-delete", nil)
	w := httptest.NewRecorder()

	server.handleDeleteLink(w, req, "test-delete")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w.Code)
	}

	// Test with auth
	req = httptest.NewRequest("DELETE", "/api/links/test-delete", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleDeleteLink(w, req, "test-delete")

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	// Verify link is deleted
	_, err = store.Get("test-delete")
	if err == nil {
		t.Error("expected error after deletion")
	}

	// Test delete non-existent link
	req = httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleDeleteLink(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}