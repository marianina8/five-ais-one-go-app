package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHandler_CreateLink(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantError   bool
		checkFields bool
	}{
		{
			name:       "valid with alias",
			body:       `{"url": "https://example.com", "alias": "my-page"}`,
			wantStatus: http.StatusCreated,
			checkFields: true,
		},
		{
			name:       "valid without alias",
			body:       `{"url": "https://example.org"}`,
			wantStatus: http.StatusCreated,
			checkFields: true,
		},
		{
			name:       "invalid JSON",
			body:       `{invalid json}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "missing url",
			body:       `{"alias": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid URL scheme",
			body:       `{"url": "ftp://example.com"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid alias too short",
			body:       `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "invalid alias forbidden name",
			body:       `{"url": "https://example.com", "alias": "api"}`,
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
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			if tt.checkFields && rr.Code == http.StatusCreated {
				var resp map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if code, ok := resp["code"].(string); !ok || code == "" {
					t.Errorf("response should contain non-empty code")
				}
				if url, ok := resp["url"].(string); !ok || url != strings.TrimSuffix(strings.TrimPrefix(tt.body, `{"url": "`), `", "alias": "my-page"}`) && url != strings.TrimSuffix(strings.TrimPrefix(tt.body, `{"url": "`), `"}`) {
					t.Errorf("response url doesn't match request")
				}
				if visits, ok := resp["visits"].(float64); !ok || visits != 0 {
					t.Errorf("response should contain visits=0")
				}
			}

			if tt.wantError && rr.Code >= 400 {
				var resp map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if _, ok := resp["error"]; !ok {
					t.Errorf("error response should contain 'error' field")
				}
			}
		})
	}
}

func TestHandler_CreateLinkDuplicateAlias(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	firstReq := httptest.NewRequest("POST", "/api/links", 
		strings.NewReader(`{"url": "https://example.com", "alias": "my-page"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRR := httptest.NewRecorder()
	handler.ServeHTTP(firstRR, firstReq)

	if firstRR.Code != http.StatusCreated {
		t.Fatalf("first request should succeed, got %d", firstRR.Code)
	}

	secondReq := httptest.NewRequest("POST", "/api/links",
		strings.NewReader(`{"url": "https://example.org", "alias": "my-page"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRR := httptest.NewRecorder()
	handler.ServeHTTP(secondRR, secondReq)

	if secondRR.Code != http.StatusConflict {
		t.Errorf("second request with same alias should return 409, got %d", secondRR.Code)
	}
}

func TestHandler_RedirectLink(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	createReq := httptest.NewRequest("POST", "/api/links",
		strings.NewReader(`{"url": "https://example.com", "alias": "test123"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("failed to create link: %d", createRR.Code)
	}

	redirectReq := httptest.NewRequest("GET", "/test123", nil)
	redirectRR := httptest.NewRecorder()
	handler.ServeHTTP(redirectRR, redirectReq)

	if redirectRR.Code != http.StatusFound {
		t.Errorf("expected redirect 302, got %d", redirectRR.Code)
	}
	if location := redirectRR.Header().Get("Location"); location != "https://example.com" {
		t.Errorf("expected Location header 'https://example.com', got %s", location)
	}

	link, err := store.Get("test123")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if link.Visits != 1 {
		t.Errorf("expected 1 visit after redirect, got %d", link.Visits)
	}
}

func TestHandler_RedirectLinkNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent link, got %d", rr.Code)
	}
}

func TestHandler_ListLinksAdminAuth(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	store.Create("test1", "https://example.com")
	store.Create("test2", "https://example.org")

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "valid admin token",
			authHeader: "Bearer admin-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid token",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing token",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed auth header",
			authHeader: "admin-token",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/links", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			if tt.wantStatus == http.StatusOK {
				var links []map[string]interface{}
				if err := json.NewDecoder(rr.Body).Decode(&links); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(links) != 2 {
					t.Errorf("expected 2 links, got %d", len(links))
				}
			}
		})
	}
}

func TestHandler_DeleteLink(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	store.Create("test123", "https://example.com")

	req := httptest.NewRequest("DELETE", "/api/links/test123", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 on successful delete, got %d", rr.Code)
	}

	redirectReq := httptest.NewRequest("GET", "/test123", nil)
	redirectRR := httptest.NewRecorder()
	handler.ServeHTTP(redirectRR, redirectReq)

	if redirectRR.Code != http.StatusNotFound {
		t.Errorf("deleted link should return 404, got %d", redirectRR.Code)
	}
}

func TestHandler_DeleteLinkNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	req := httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent link, got %d", rr.Code)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(filepath.Join(tmpDir, "test.json"))
	defer store.Close()

	handler := NewHandler(store, "admin-token")

	tests := []struct {
		method string
		path   string
	}{
		{"PUT", "/api/links"},
		{"PATCH", "/api/links"},
		{"POST", "/test123"},
		{"DELETE", "/test123"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405 for %s %s, got %d", tt.method, tt.path, rr.Code)
			}
		})
	}
}

func TestRandomCodeGeneration(t *testing.T) {
	code := generateRandomCode(7)
	if len(code) != 7 {
		t.Errorf("expected code length 7, got %d", len(code))
	}

	validChars := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	if !validChars.MatchString(code) {
		t.Errorf("code contains invalid characters: %s", code)
	}
}