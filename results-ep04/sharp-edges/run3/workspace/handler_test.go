package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func setupTestHandler(t *testing.T) (*Handler, func()) {
	tmpFile, err := os.CreateTemp("", "shortener-handler-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(store, "test-admin-token")

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return handler, cleanup
}

func TestHandler_CreateLink(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "valid request with alias",
			body:       `{"url": "https://example.com", "alias": "my-page"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid request without alias",
			body:       `{"url": "https://example.com"}`,
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
			body:       `{"alias": "my-page"}`,
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
			name:       "alias is 'api'",
			body:       `{"url": "https://example.com", "alias": "api"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "alias with invalid characters",
			body:       `{"url": "https://example.com", "alias": "my@page"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid JSON",
			body:       `{"url": "https://example.com"`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/links", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("Status code = %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantError {
				var resp ErrorResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Errorf("Failed to parse error response: %v", err)
				}
				if resp.Error == "" {
					t.Error("Expected error message")
				}
			} else if rr.Code == http.StatusCreated {
				var link Link
				if err := json.Unmarshal(rr.Body.Bytes(), &link); err != nil {
					t.Errorf("Failed to parse response: %v", err)
				}
				if link.URL != "https://example.com" {
					t.Errorf("URL = %s, want %s", link.URL, "https://example.com")
				}
				if tt.body == `{"url": "https://example.com"}` {
					if len(link.Code) != 7 {
						t.Errorf("Generated code length = %d, want 7", len(link.Code))
					}
				}
			}
		})
	}
}

func TestHandler_CreateLinkDuplicateAlias(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// First request
	body1 := `{"url": "https://example1.com", "alias": "my-page"}`
	req1 := httptest.NewRequest("POST", "/api/links", strings.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusCreated {
		t.Fatalf("First create failed: status = %d", rr1.Code)
	}

	// Second request with same alias
	body2 := `{"url": "https://example2.com", "alias": "my-page"}`
	req2 := httptest.NewRequest("POST", "/api/links", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusConflict {
		t.Errorf("Status code = %d, want %d", rr2.Code, http.StatusConflict)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rr2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.Error), "already") {
		t.Errorf("Error message doesn't indicate conflict: %s", resp.Error)
	}
}

func TestHandler_FollowLink(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create a link first
	createReq := httptest.NewRequest("POST", "/api/links", 
		strings.NewReader(`{"url": "https://example.com", "alias": "test123"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("Create failed: status = %d", createRR.Code)
	}

	// Follow the link
	followReq := httptest.NewRequest("GET", "/test123", nil)
	followRR := httptest.NewRecorder()
	handler.ServeHTTP(followRR, followReq)

	if followRR.Code != http.StatusFound {
		t.Errorf("Status code = %d, want %d", followRR.Code, http.StatusFound)
	}

	location := followRR.Header().Get("Location")
	if location != "https://example.com" {
		t.Errorf("Location = %s, want %s", location, "https://example.com")
	}
}

func TestHandler_FollowLinkNotFound(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("Status code = %d, want %d", rr.Code, http.StatusNotFound)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.Error), "not found") {
		t.Errorf("Error message doesn't indicate not found: %s", resp.Error)
	}
}

func TestHandler_ListLinksUnauthorized(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/links", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Status code = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandler_ListLinksAuthorized(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create some links
	links := []string{
		`{"url": "https://example1.com", "alias": "link1"}`,
		`{"url": "https://example2.com", "alias": "link2"}`,
	}

	for _, body := range links {
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// List links with auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Status code = %d, want %d", rr.Code, http.StatusOK)
	}

	var list []Link
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("Got %d links, want 2", len(list))
	}
}

func TestHandler_DeleteLink(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create a link
	createReq := httptest.NewRequest("POST", "/api/links", 
		strings.NewReader(`{"url": "https://example.com", "alias": "test123"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)

	// Delete with auth
	deleteReq := httptest.NewRequest("DELETE", "/api/links/test123", nil)
	deleteReq.Header.Set("Authorization", "Bearer test-admin-token")
	deleteRR := httptest.NewRecorder()
	handler.ServeHTTP(deleteRR, deleteReq)

	if deleteRR.Code != http.StatusNoContent {
		t.Errorf("Status code = %d, want %d", deleteRR.Code, http.StatusNoContent)
	}

	// Verify link is gone
	followReq := httptest.NewRequest("GET", "/test123", nil)
	followRR := httptest.NewRecorder()
	handler.ServeHTTP(followRR, followReq)

	if followRR.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", followRR.Code, http.StatusNotFound)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	tests := []struct {
		method string
		path   string
	}{
		{"PUT", "/api/links"},
		{"PATCH", "/api/links"},
		{"PUT", "/api/links/test"},
		{"PATCH", "/api/links/test"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("Status code = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestHandler_LargeRequestBody(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create a body larger than 1 MiB
	largeBody := make([]byte, 2<<20) // 2 MiB
	for i := range largeBody {
		largeBody[i] = 'A'
	}
	largeBody[0] = '{'
	largeBody[1] = '"'
	largeBody[2] = 'u'
	largeBody[3] = 'r'
	largeBody[4] = 'l'
	largeBody[5] = '"'
	largeBody[6] = ':'
	largeBody[7] = '"'
	largeBody[len(largeBody)-2] = '"'
	largeBody[len(largeBody)-1] = '}'

	req := httptest.NewRequest("POST", "/api/links", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Status code = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}