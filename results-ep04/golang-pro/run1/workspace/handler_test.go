package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHandler_CreateLink(t *testing.T) {
	tempFile := "test_handler.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Test valid request without alias
	reqBody := `{"url": "https://example.com"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var resp CreateLinkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Code == "" {
		t.Error("Expected non-empty code")
	}
	if resp.URL != "https://example.com" {
		t.Errorf("Expected URL https://example.com, got %s", resp.URL)
	}
	if resp.Visits != 0 {
		t.Errorf("Expected 0 visits, got %d", resp.Visits)
	}

	// Test valid request with alias
	reqBody = `{"url": "https://google.com", "alias": "google"}`
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	// Test invalid URL
	reqBody = `{"url": "ftp://example.com"}`
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid URL, got %d", w.Code)
	}

	// Test invalid alias
	reqBody = `{"url": "https://example.com", "alias": "ab"}`
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid alias, got %d", w.Code)
	}

	// Test duplicate alias
	reqBody = `{"url": "https://example2.com", "alias": "google"}`
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409 for duplicate alias, got %d", w.Code)
	}
}

func TestHandler_Redirect(t *testing.T) {
	tempFile := "test_redirect.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// First create a link
	link, err := store.CreateLink("https://example.com", "test-alias")
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Test redirect by code
	req := httptest.NewRequest("GET", "/"+link.Code, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if location != "https://example.com" {
		t.Errorf("Expected Location header https://example.com, got %s", location)
	}

	// Test redirect by alias
	req = httptest.NewRequest("GET", "/test-alias", nil)
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("Expected status 302 for alias, got %d", w.Code)
	}

	// Test non-existent link
	req = httptest.NewRequest("GET", "/nonexistent", nil)
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent link, got %d", w.Code)
	}
}

func TestHandler_ListLinks(t *testing.T) {
	tempFile := "test_list.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create some links
	store.CreateLink("https://example1.com", "")
	store.CreateLink("https://example2.com", "")

	// Test without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 without auth, got %d", w.Code)
	}

	// Test with auth
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 with auth, got %d", w.Code)
	}

	var links []Link
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("Expected 2 links, got %d", len(links))
	}

	// Verify sorting (oldest first)
	for i := 1; i < len(links); i++ {
		if links[i].CreatedAt.Before(links[i-1].CreatedAt) {
			t.Error("Links should be sorted by CreatedAt (oldest first)")
		}
	}
}

func TestHandler_DeleteLink(t *testing.T) {
	tempFile := "test_delete.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create a link
	link, err := store.CreateLink("https://example.com", "test-delete")
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Test delete without auth
	req := httptest.NewRequest("DELETE", "/api/links/"+link.Code, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 without auth, got %d", w.Code)
	}

	// Test delete with auth
	req = httptest.NewRequest("DELETE", "/api/links/"+link.Code, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	// Verify link is gone
	req = httptest.NewRequest("GET", "/"+link.Code, nil)
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 after deletion, got %d", w.Code)
	}

	// Test delete non-existent link
	req = httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 for non-existent link, got %d", w.Code)
	}
}

func TestHandler_AdminAuth(t *testing.T) {
	tempFile := "test_auth.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	testCases := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{"No auth header", "", http.StatusUnauthorized},
		{"Invalid format", "InvalidFormat", http.StatusUnauthorized},
		{"Wrong scheme", "Basic test-token", http.StatusUnauthorized},
		{"Wrong token", "Bearer wrong-token", http.StatusUnauthorized},
		{"Correct token", "Bearer test-token", http.StatusOK},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/links", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d for auth '%s', got %d", tc.expectedStatus, tc.authHeader, w.Code)
			}
		})
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	tempFile := "test_methods.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create a link first
	link, err := store.CreateLink("https://example.com", "")
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	testCases := []struct {
		method string
		path   string
	}{
		{"PUT", "/api/links"},
		{"PATCH", "/api/links"},
		{"PUT", "/api/links/" + link.Code},
		{"PATCH", "/api/links/" + link.Code},
		{"POST", "/" + link.Code},
		{"PUT", "/" + link.Code},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s %s", tc.method, tc.path), func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for %s %s, got %d", tc.method, tc.path, w.Code)
			}
		})
	}
}