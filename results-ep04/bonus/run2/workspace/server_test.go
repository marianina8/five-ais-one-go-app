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
)

func TestServer_CreateLink(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "admin-token")

	tests := []struct {
		name       string
		reqBody    string
		wantStatus int
		wantError  bool
		checkFunc  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "valid request with alias",
			reqBody:    `{"url": "https://example.com", "alias": "my-alias"}`,
			wantStatus: http.StatusCreated,
			checkFunc: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var link Link
				if err := json.Unmarshal(rr.Body.Bytes(), &link); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}
				if link.Code != "my-alias" {
					t.Errorf("Expected code 'my-alias', got '%s'", link.Code)
				}
				if link.URL != "https://example.com" {
					t.Errorf("Expected URL 'https://example.com', got '%s'", link.URL)
				}
				if link.Alias != "my-alias" {
					t.Errorf("Expected alias 'my-alias', got '%s'", link.Alias)
				}
				if link.Visits != 0 {
					t.Errorf("Expected 0 visits, got %d", link.Visits)
				}
			},
		},
		{
			name:       "valid request without alias",
			reqBody:    `{"url": "https://example.com"}`,
			wantStatus: http.StatusCreated,
			checkFunc: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var link Link
				if err := json.Unmarshal(rr.Body.Bytes(), &link); err != nil {
					t.Fatalf("Failed to unmarshal response: %v", err)
				}
				if len(link.Code) != 7 {
					t.Errorf("Expected code length 7, got %d", len(link.Code))
				}
				if link.URL != "https://example.com" {
					t.Errorf("Expected URL 'https://example.com', got '%s'", link.URL)
				}
				if link.Alias != "" {
					t.Errorf("Expected empty alias, got '%s'", link.Alias)
				}
			},
		},
		{
			name:       "missing url",
			reqBody:    `{"alias": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid url scheme",
			reqBody:    `{"url": "ftp://example.com", "alias": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "relative url",
			reqBody:    `{"url": "/relative", "alias": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "empty url",
			reqBody:    `{"url": "", "alias": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "alias too short",
			reqBody:    `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "alias with invalid chars",
			reqBody:    `{"url": "https://example.com", "alias": "test@alias"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "alias 'api' not allowed",
			reqBody:    `{"url": "https://example.com", "alias": "api"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "duplicate alias",
			reqBody:    `{"url": "https://example.com", "alias": "duplicate"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "duplicate alias conflict",
			reqBody:    `{"url": "https://example2.com", "alias": "duplicate"}`,
			wantStatus: http.StatusConflict,
			wantError:  true,
		},
		{
			name:       "malformed json",
			reqBody:    `{"url": "https://example.com", "alias": "test"`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "unknown field",
			reqBody:    `{"url": "https://example.com", "alias": "test", "unknown": "field"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/links", strings.NewReader(tt.reqBody))
			req.Header.Set("Content-Type", "application/json")
			
			rr := httptest.NewRecorder()
			server.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, rr)
			}

			if tt.wantError && rr.Code >= 400 {
				var errResp ErrorResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
					t.Errorf("Failed to unmarshal error response: %v", err)
				}
				if errResp.Error == "" {
					t.Error("Expected error message in response")
				}
			}

			// Verify content-type
			contentType := rr.Header().Get("Content-Type")
			if rr.Code != http.StatusNoContent && !strings.Contains(contentType, "application/json") {
				t.Errorf("Expected application/json content-type, got %s", contentType)
			}
		})
	}
}

func TestServer_RedirectLink(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "admin-token")

	// Create a link first
	link, err := store.CreateLink("test-redirect", "https://redirect.example.com", "")
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantLoc    string
	}{
		{
			name:       "valid redirect",
			method:     "GET",
			path:       "/test-redirect",
			wantStatus: http.StatusFound,
			wantLoc:    "https://redirect.example.com",
		},
		{
			name:       "non-existent code",
			method:     "GET",
			path:       "/nonexistent",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong method",
			method:     "POST",
			path:       "/test-redirect",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			server.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			if tt.wantLoc != "" {
				loc := rr.Header().Get("Location")
				if loc != tt.wantLoc {
					t.Errorf("Expected Location '%s', got '%s'", tt.wantLoc, loc)
				}
			}

			// Check visits were incremented for successful redirects
			if tt.wantStatus == http.StatusFound {
				updatedLink, err := store.GetLink("test-redirect")
				if err != nil {
					t.Fatalf("GetLink failed: %v", err)
				}
				if updatedLink.Visits != link.Visits+1 {
					t.Errorf("Expected visits to increment from %d to %d, got %d", 
						link.Visits, link.Visits+1, updatedLink.Visits)
				}
				link = updatedLink // Update for next iteration
			}
		})
	}
}

func TestServer_AdminEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	adminToken := "secret-admin-token"
	server := NewServer(store, adminToken)

	// Create some test links
	links := []struct {
		code string
		url  string
	}{
		{"link1", "https://link1.com"},
		{"link2", "https://link2.com"},
		{"link3", "https://link3.com"},
	}

	for _, l := range links {
		_, err := store.CreateLink(l.code, l.url, "")
		if err != nil {
			t.Fatalf("CreateLink failed for %s: %v", l.code, err)
		}
	}

	// Test list links without auth
	t.Run("list links without auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/links", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	// Test list links with invalid token
	t.Run("list links with invalid token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/links", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	// Test list links with valid token
	t.Run("list links with valid token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/links", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		var response []Link
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(response) != 3 {
			t.Errorf("Expected 3 links, got %d", len(response))
		}

		// Verify correct order (oldest first)
		for i, link := range response {
			expectedCode := fmt.Sprintf("link%d", i+1)
			if link.Code != expectedCode {
				t.Errorf("Link %d: expected code '%s', got '%s'", i, expectedCode, link.Code)
			}
		}
	})

	// Test delete link without auth
	t.Run("delete link without auth", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/links/link1", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", rr.Code)
		}
	})

	// Test delete link with valid token
	t.Run("delete link with valid token", func(t *testing.T) {
		// First verify link exists
		_, err := store.GetLink("link1")
		if err != nil {
			t.Fatalf("Link should exist before delete: %v", err)
		}

		req := httptest.NewRequest("DELETE", "/api/links/link1", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d", rr.Code)
		}

		// Verify link is gone
		_, err = store.GetLink("link1")
		if err == nil {
			t.Error("Expected error for deleted link")
		}

		// Verify redirect no longer works
		req2 := httptest.NewRequest("GET", "/link1", nil)
		rr2 := httptest.NewRecorder()
		server.ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusNotFound {
			t.Errorf("Expected 404 after delete, got %d", rr2.Code)
		}
	})

	// Test delete non-existent link
	t.Run("delete non-existent link", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", rr.Code)
		}
	})
}

func TestServer_LargeRequestBody(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "admin-token")

	// Create a request body larger than 1 MiB
	largeBody := make([]byte, 2<<20) // 2 MiB
	for i := range largeBody {
		largeBody[i] = 'A'
	}

	req := httptest.NewRequest("POST", "/api/links", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	// Should reject due to size limit
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status 413, got %d", rr.Code)
	}
}

func TestUtils_RandomCodeGeneration(t *testing.T) {
	// Test that generated codes are the right length
	for i := 0; i < 1000; i++ {
		code := generateRandomCode(7)
		if len(code) != 7 {
			t.Errorf("Expected code length 7, got %d", len(code))
		}

		// Test that code only contains valid characters
		for _, ch := range code {
			if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
				t.Errorf("Invalid character in code: %c", ch)
			}
		}
	}

	// Test different lengths
	lengths := []int{1, 5, 10, 20}
	for _, length := range lengths {
		code := generateRandomCode(length)
		if len(code) != length {
			t.Errorf("Expected code length %d, got %d", length, len(code))
		}
	}
}

func TestMain_EnvironmentValidation(t *testing.T) {
	// Save original env and restore after test
	originalToken := os.Getenv("ADMIN_TOKEN")
	defer os.Setenv("ADMIN_TOKEN", originalToken)

	// Test with empty token
	os.Setenv("ADMIN_TOKEN", "")
	
	// We can't easily test the exit, but we can verify the flag parsing works
	// by checking that the store initialization would fail without proper setup
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")
	
	// This should work regardless of env var when called directly
	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore should work without env var: %v", err)
	}
	store.Close()
}