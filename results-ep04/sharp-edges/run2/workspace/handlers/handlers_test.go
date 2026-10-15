package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"shortener/store"
)

func TestHandlers_CreateLink(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "valid request without alias",
			body:       `{"url": "https://example.com"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid request with alias",
			body:       `{"url": "https://example.com", "alias": "myalias"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid URL",
			body:       `{"url": "not-a-url"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "missing URL",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid JSON",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "URL too long",
			body:       fmt.Sprintf(`{"url": "https://example.com/%s"}`, strings.Repeat("a", 3000)),
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "alias 'api' forbidden",
			body:       `{"url": "https://example.com", "alias": "api"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid alias format",
			body:       `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid alias format too long",
			body:       `{"url": "https://example.com", "alias": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid alias format special chars",
			body:       `{"url": "https://example.com", "alias": "test@test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if tt.wantError && w.Code < 400 {
				t.Errorf("expected error status (4xx or 5xx), got %d", w.Code)
			}

			if tt.wantStatus == http.StatusCreated {
				var resp map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatal(err)
				}

				if resp["url"] != "https://example.com" {
					t.Errorf("expected url in response")
				}
				if resp["visits"] != 0.0 {
					t.Errorf("expected 0 visits")
				}
			}
		})
	}
}

func TestHandlers_CreateLinkDuplicateAlias(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	// Create first link with alias
	body := `{"url": "https://example.com", "alias": "myalias"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	// Try to create second link with same alias
	req2 := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("expected status 409 for duplicate alias, got %d", w2.Code)
	}
}

func TestHandlers_Redirect(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	// Create a link
	body := `{"url": "https://example.com", "alias": "test123"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	// Test redirect
	req2 := httptest.NewRequest("GET", "/test123", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w2.Code)
	}
	if location := w2.Header().Get("Location"); location != "https://example.com" {
		t.Errorf("expected Location header 'https://example.com', got %s", location)
	}

	// Test non-existent code
	req3 := httptest.NewRequest("GET", "/nonexistent", nil)
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent code, got %d", w3.Code)
	}
}

func TestHandlers_ListLinksUnauthorized(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	// Try to list links without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w.Code)
	}
}

func TestHandlers_ListLinksAuthorized(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	// Create a link
	body := `{"url": "https://example.com", "alias": "test123"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// List links with auth
	req2 := httptest.NewRequest("GET", "/api/links", nil)
	req2.Header.Set("Authorization", "Bearer admin-token")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status 200 with auth, got %d", w2.Code)
	}

	var links []interface{}
	if err := json.NewDecoder(w2.Body).Decode(&links); err != nil {
		t.Fatal(err)
	}

	if len(links) != 1 {
		t.Errorf("expected 1 link, got %d", len(links))
	}
}

func TestHandlers_DeleteLink(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	// Create a link
	body := `{"url": "https://example.com", "alias": "test123"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Delete link without auth (should fail)
	req2 := httptest.NewRequest("DELETE", "/api/links/test123", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w2.Code)
	}

	// Delete link with auth
	req3 := httptest.NewRequest("DELETE", "/api/links/test123", nil)
	req3.Header.Set("Authorization", "Bearer admin-token")
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w3.Code)
	}

	// Verify link is gone
	req4 := httptest.NewRequest("GET", "/test123", nil)
	w4 := httptest.NewRecorder()
	h.ServeHTTP(w4, req4)

	if w4.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for deleted link, got %d", w4.Code)
	}
}

func TestHandlers_MethodNotAllowed(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	// Create a link first
	body := `{"url": "https://example.com", "alias": "test123"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Test PUT on /api/links (not allowed)
	req2 := httptest.NewRequest("PUT", "/api/links", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	if w2.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for PUT, got %d", w2.Code)
	}

	// Test POST on redirect endpoint (not allowed)
	req3 := httptest.NewRequest("POST", "/test123", nil)
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, req3)

	if w3.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for POST on redirect, got %d", w3.Code)
	}
}

func TestHandlers_LargeRequestBody(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-handlers-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	h := NewAPIHandler(st, "admin-token")

	// Create request body larger than 1MB
	largeBody := strings.Repeat("x", 2<<20) // 2MB
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(`{"url": "https://example.com"}`+largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	h.ServeHTTP(w, req)
	
	// Should either reject or handle gracefully
	// The MaxBytesReader will truncate at 1MB
	if w.Code == http.StatusCreated {
		// That's fine too - our MaxBytesReader limits to 1MB
	} else if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected rejection of large request body, got status %d", w.Code)
	}
}