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
	// Create a test store
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create handler
	handler := NewHandler(store, "test-token")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		checkBody  bool
	}{
		{
			name:       "valid request without alias",
			body:       `{"url": "https://example.com"}`,
			wantStatus: http.StatusCreated,
			checkBody:  true,
		},
		{
			name:       "valid request with alias",
			body:       `{"url": "https://example.com", "alias": "my-page"}`,
			wantStatus: http.StatusCreated,
			checkBody:  true,
		},
		{
			name:       "invalid URL",
			body:       `{"url": "ftp://example.com"}`,
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name:       "invalid alias too short",
			body:       `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name:       "invalid alias chars",
			body:       `{"url": "https://example.com", "alias": "ab!cd"}`,
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name:       "alias is api",
			body:       `{"url": "https://example.com", "alias": "api"}`,
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name:       "duplicate alias",
			body:       `{"url": "https://example.com", "alias": "duplicate"}`,
			wantStatus: http.StatusCreated,
			checkBody:  true,
		},
		{
			name:       "second request with same alias",
			body:       `{"url": "https://example.com/2", "alias": "duplicate"}`,
			wantStatus: http.StatusConflict,
			checkBody:  false,
		},
		{
			name:       "malformed JSON",
			body:       `{"url": "https://example.com",`,
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
		{
			name:       "empty body",
			body:       ``,
			wantStatus: http.StatusBadRequest,
			checkBody:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/links", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.checkBody && tt.wantStatus == http.StatusCreated {
				var response Link
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if response.URL == "" {
					t.Error("response missing URL")
				}
				if response.Code == "" {
					t.Error("response missing code")
				}
				if response.CreatedAt.IsZero() {
					t.Error("response missing created_at")
				}
				if response.Visits != 0 {
					t.Error("response visits should be 0")
				}
			}
		})
	}
}

func TestHandler_FollowLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create a link first
	link, err := store.CreateLink("https://example.com", "test-code")
	if err != nil {
		t.Fatal(err)
	}

	// Test successful redirect
	req := httptest.NewRequest("GET", "/"+link.Code, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != link.URL {
		t.Errorf("Location = %q, want %q", loc, link.URL)
	}

	// Test non-existent code
	req = httptest.NewRequest("GET", "/non-existent", nil)
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status for non-existent = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_ListLinks(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create some links
	store.CreateLink("https://example.com/1", "first")
	store.CreateLink("https://example.com/2", "second")

	// Test without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status without auth = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Test with wrong auth
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status with wrong auth = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Test with correct auth
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status with auth = %d, want %d", w.Code, http.StatusOK)
	}

	var links []Link
	if err := json.NewDecoder(w.Body).Decode(&links); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("got %d links, want 2", len(links))
	}
}

func TestHandler_DeleteLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create a link
	link, err := store.CreateLink("https://example.com", "test-delete")
	if err != nil {
		t.Fatal(err)
	}

	// Test delete without auth
	req := httptest.NewRequest("DELETE", "/api/links/"+link.Code, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status without auth = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Test delete with auth
	req = httptest.NewRequest("DELETE", "/api/links/"+link.Code, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify link is deleted
	_, err = store.GetAndIncrement(link.Code)
	if err == nil {
		t.Error("link should be deleted")
	}

	// Test delete non-existent link with auth
	req = httptest.NewRequest("DELETE", "/api/links/non-existent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status for non-existent = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Test PUT on /api/links (not allowed)
	req := httptest.NewRequest("PUT", "/api/links", strings.NewReader(`{"url":"https://example.com"}`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}

	// Test POST on /api/links/{code} (not allowed)
	req = httptest.NewRequest("POST", "/api/links/some-code", nil)
	w = httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST on specific link status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandler_RequestBodyLimit(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create valid JSON request body larger than 1MB
	// Build a valid JSON object with a long URL field
	urlValue := `"https://example.com/` + strings.Repeat("a", 2<<20) + `"`
	jsonBody := `{"url":` + urlValue + `}`
	
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should be 413 Payload Too Large
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestIsAdmin(t *testing.T) {
	h := &handler{adminToken: "secret-token"}

	tests := []struct {
		authHeader string
		want       bool
	}{
		{"Bearer secret-token", true},
		{"Bearer wrong-token", false},
		{"Bearer", false},
		{"Bearer ", false},
		{"", false},
		{"Bearer ", false},
		{"Basic secret-token", false},
		{"Bearer secret-token extra", false},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/links", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			got := h.isAdmin(req)
			if got != tt.want {
				t.Errorf("isAdmin(%q) = %v, want %v", tt.authHeader, got, tt.want)
			}
		})
	}
}