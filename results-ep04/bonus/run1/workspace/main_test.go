package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestJSONStoreCreate(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Test creating link without alias
	link1, err := store.Create("https://example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(link1.Code) != 7 {
		t.Errorf("expected code length 7, got %d", len(link1.Code))
	}
	if link1.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", link1.URL)
	}
	if link1.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link1.Visits)
	}

	// Test creating link with alias
	link2, err := store.Create("https://google.com", "google")
	if err != nil {
		t.Fatal(err)
	}
	if link2.Code != "google" {
		t.Errorf("expected code 'google', got %s", link2.Code)
	}

	// Test duplicate alias
	_, err = store.Create("https://example.org", "google")
	if err == nil || !strings.Contains(err.Error(), "alias already taken") {
		t.Errorf("expected 'alias already taken' error, got %v", err)
	}

	// Test invalid URL
	_, err = store.Create("ftp://example.com", "")
	if err == nil || !strings.Contains(err.Error(), "URL scheme must be http or https") {
		t.Errorf("expected URL validation error, got %v", err)
	}

	// Test URL too long
	longURL := "https://example.com/" + strings.Repeat("a", 3000)
	_, err = store.Create(longURL, "")
	if err == nil || !strings.Contains(err.Error(), "URL too long") {
		t.Errorf("expected 'URL too long' error, got %v", err)
	}
}

func TestJSONStoreOperations(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a link
	_, err = store.Create("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Test Get
	retrieved, err := store.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", retrieved.URL)
	}

	// Test IncrementVisits
	err = store.IncrementVisits("test")
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err = store.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if retrieved.Visits != 1 {
		t.Errorf("expected 1 visit after increment, got %d", retrieved.Visits)
	}

	// Test List
	links, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Errorf("expected 1 link, got %d", len(links))
	}
	if links[0].Code != "test" {
		t.Errorf("expected link code 'test', got %s", links[0].Code)
	}

	// Test Delete
	err = store.Delete("test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Get("test")
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestCreateLinkEndpoint(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "Valid POST without alias",
			method:     "POST",
			path:       "/api/links",
			body:       `{"url": "https://example.com"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "Valid POST with alias",
			method:     "POST",
			path:       "/api/links",
			body:       `{"url": "https://google.com", "alias": "google"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "Invalid URL scheme",
			method:     "POST",
			path:       "/api/links",
			body:       `{"url": "ftp://example.com"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "URL scheme must be http or https",
		},
		{
			name:       "Missing URL",
			method:     "POST",
			path:       "/api/links",
			body:       `{"alias": "test"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid JSON",
		},
		{
			name:       "Invalid alias (too short)",
			method:     "POST",
			path:       "/api/links",
			body:       `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "alias must match",
		},
		{
			name:       "Reserved alias",
			method:     "POST",
			path:       "/api/links",
			body:       `{"url": "https://example.com", "alias": "api"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "alias 'api' is reserved",
		},
		{
			name:       "Duplicate alias",
			method:     "POST",
			path:       "/api/links",
			body:       `{"url": "https://example.com", "alias": "google"}`,
			wantStatus: http.StatusConflict,
			wantError:  "alias already taken",
		},
		{
			name:       "Unknown field",
			method:     "POST",
			path:       "/api/links",
			body:       `{"url": "https://example.com", "unknown": "field"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if tt.wantError != "" {
				var resp map[string]string
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(resp["error"], tt.wantError) {
					t.Errorf("expected error containing %q, got %q", tt.wantError, resp["error"])
				}
			}

			if tt.wantStatus == http.StatusCreated {
				var link Link
				if err := json.NewDecoder(w.Body).Decode(&link); err != nil {
					t.Fatal(err)
				}
				if link.URL == "" {
					t.Error("expected link in response")
				}
			}
		})
	}
}

func TestRedirectEndpoint(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a link first
	_, err = store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(store, "test-token")

	// Test valid redirect
	req := httptest.NewRequest("GET", "/test123", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", w.Code)
	}
	if location := w.Header().Get("Location"); location != "https://example.com" {
		t.Errorf("expected Location header https://example.com, got %s", location)
	}

	// Test non-existent code
	req = httptest.NewRequest("GET", "/nonexistent", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestAdminEndpoints(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create some links
	_, err = store.Create("https://example1.com", "link1")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond) // Ensure different CreatedAt
	_, err = store.Create("https://example2.com", "link2")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(store, "test-token")

	// Test list links without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w.Code)
	}

	// Test list links with auth
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 with auth, got %d", w.Code)
	}

	var links []Link
	if err := json.NewDecoder(w.Body).Decode(&links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}
	// Should be sorted oldest first
	if links[0].Code != "link1" || links[1].Code != "link2" {
		t.Errorf("expected links sorted by creation time")
	}

	// Test delete link without auth
	req = httptest.NewRequest("DELETE", "/api/links/link1", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", w.Code)
	}

	// Test delete link with auth
	req = httptest.NewRequest("DELETE", "/api/links/link1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	// Verify link was deleted
	_, err = store.Get("link1")
	if err == nil {
		t.Error("expected error after deletion")
	}

	// Test delete non-existent link
	req = httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent link, got %d", w.Code)
	}
}

func TestErrorResponses(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, "test-token")

	// Test not found
	req := httptest.NewRequest("GET", "/unknown/path", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestValidationFunctions(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		alias     string
		shouldErr bool
		errMsg    string
	}{
		{"Valid HTTPS", "https://example.com", "", false, ""},
		{"Valid HTTP", "http://example.com", "", false, ""},
		{"Invalid scheme", "ftp://example.com", "", true, "URL scheme must be http or https"},
		{"Missing host", "https://", "", true, "URL must have a host"},
		{"URL too long", "https://example.com/" + strings.Repeat("a", 3000), "", true, "URL too long"},
		{"Valid alias", "https://example.com", "my-page_123", false, ""},
		{"Invalid alias short", "https://example.com", "ab", true, "alias must match"},
		{"Invalid alias long", "https://example.com", strings.Repeat("a", 33), true, "alias must match"},
		{"Invalid alias chars", "https://example.com", "my page", true, "alias must match"},
		{"Reserved alias", "https://example.com", "api", true, "alias 'api' is reserved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.url != "" {
				err := ValidateURL(tt.url)
				if tt.shouldErr && err == nil {
					t.Error("expected error but got none")
				}
				if !tt.shouldErr && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.shouldErr && err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			}

			if tt.alias != "" {
				err := ValidateAlias(tt.alias)
				if tt.shouldErr && err == nil {
					t.Error("expected error but got none")
				}
				if !tt.shouldErr && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if tt.shouldErr && err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}