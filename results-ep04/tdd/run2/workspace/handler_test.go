package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestHandler_CreateLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := newHandler(store, "test-token")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    bool
	}{
		{
			name:       "valid request without alias",
			body:       `{"url": "https://example.com"}`,
			wantStatus: http.StatusCreated,
			wantErr:    false,
		},
		{
			name:       "valid request with alias",
			body:       `{"url": "https://example.com", "alias": "my-page"}`,
			wantStatus: http.StatusCreated,
			wantErr:    false,
		},
		{
			name:       "invalid URL",
			body:       `{"url": "ftp://example.com"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "invalid alias",
			body:       `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "alias is api",
			body:       `{"url": "https://example.com", "alias": "api"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "missing url",
			body:       `{"alias": "my-page"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "duplicate alias",
			body:       `{"url": "https://example2.com", "alias": "my-page"}`,
			wantStatus: http.StatusConflict,
			wantErr:    true,
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

			respBody, _ := io.ReadAll(w.Body)
			if tt.wantErr {
				var errResp map[string]string
				if err := json.Unmarshal(respBody, &errResp); err != nil {
					t.Errorf("failed to parse error response: %v", err)
				}
				if !strings.Contains(errResp["error"], "error") {
					t.Errorf("error response missing error field: %s", respBody)
				}
			} else {
				var link Link
				if err := json.Unmarshal(respBody, &link); err != nil {
					t.Errorf("failed to parse response: %v", err)
				}
				if link.URL != "https://example.com" && link.URL != "https://example2.com" {
					t.Errorf("URL mismatch: %s", link.URL)
				}
			}
		})
	}
}

func TestHandler_GetLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := newHandler(store, "test-token")

	// Create a link first
	body := `{"url": "https://example.com", "alias": "test-link"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create link: %d", w.Code)
	}

	// Now test redirect
	req2 := httptest.NewRequest("GET", "/test-link", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusFound {
		t.Errorf("redirect status = %d, want %d", w2.Code, http.StatusFound)
	}
	location := w2.Header().Get("Location")
	if location != "https://example.com" {
		t.Errorf("Location = %s, want https://example.com", location)
	}

	// Test non-existent link
	req3 := httptest.NewRequest("GET", "/non-existent", nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNotFound {
		t.Errorf("non-existent status = %d, want %d", w3.Code, http.StatusNotFound)
	}
}

func TestHandler_ListLinks(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := newHandler(store, "test-token")

	// Create some links
	links := []string{
		`{"url": "https://example.com/1", "alias": "link1"}`,
		`{"url": "https://example.com/2", "alias": "link2"}`,
	}

	for _, body := range links {
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("failed to create link: %d", w.Code)
		}
	}

	// Test list without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Test list with auth
	req2 := httptest.NewRequest("GET", "/api/links", nil)
	req2.Header.Set("Authorization", "Bearer test-token")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("auth status = %d, want %d", w2.Code, http.StatusOK)
	}

	var linksResp []Link
	if err := json.NewDecoder(w2.Body).Decode(&linksResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(linksResp) != 2 {
		t.Errorf("got %d links, want 2", len(linksResp))
	}
}

func TestHandler_DeleteLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := newHandler(store, "test-token")

	// Create a link
	body := `{"url": "https://example.com", "alias": "to-delete"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create link: %d", w.Code)
	}

	// Delete without auth
	req2 := httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Errorf("no auth status = %d, want %d", w2.Code, http.StatusUnauthorized)
	}

	// Delete with auth
	req3 := httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	req3.Header.Set("Authorization", "Bearer test-token")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNoContent {
		t.Errorf("auth status = %d, want %d", w3.Code, http.StatusNoContent)
	}

	// Verify link is gone
	req4 := httptest.NewRequest("GET", "/to-delete", nil)
	w4 := httptest.NewRecorder()
	handler.ServeHTTP(w4, req4)

	if w4.Code != http.StatusNotFound {
		t.Errorf("deleted link status = %d, want %d", w4.Code, http.StatusNotFound)
	}

	// Delete non-existent with auth
	req5 := httptest.NewRequest("DELETE", "/api/links/non-existent", nil)
	req5.Header.Set("Authorization", "Bearer test-token")
	w5 := httptest.NewRecorder()
	handler.ServeHTTP(w5, req5)

	if w5.Code != http.StatusNotFound {
		t.Errorf("non-existent status = %d, want %d", w5.Code, http.StatusNotFound)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := newHandler(store, "test-token")

	// Create a link first
	body := `{"url": "https://example.com", "alias": "test"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create link: %d", w.Code)
	}

	// Test PUT on /api/links (should be 404)
	req2 := httptest.NewRequest("PUT", "/api/links", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("PUT status = %d, want %d", w2.Code, http.StatusNotFound)
	}

	// Test POST on /{code} (should be 404)
	req3 := httptest.NewRequest("POST", "/test", nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code != http.StatusNotFound {
		t.Errorf("POST redirect status = %d, want %d", w3.Code, http.StatusNotFound)
	}
}

func TestHandler_RequestSizeLimit(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := newHandler(store, "test-token")

	// Create a very large request body (> 1 MiB)
	largeBody := bytes.Repeat([]byte("a"), 2<<20) // 2 MiB
	req := httptest.NewRequest("POST", "/api/links", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should get 413 Payload Too Large
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("large body status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := newHandler(store, "test-token")

	// Invalid JSON
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(`{"url": "https://example.com"`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	// Unknown fields
	req2 := httptest.NewRequest("POST", "/api/links", strings.NewReader(`{"url": "https://example.com", "unknown": "field"}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("unknown fields status = %d, want %d", w2.Code, http.StatusBadRequest)
	}
}