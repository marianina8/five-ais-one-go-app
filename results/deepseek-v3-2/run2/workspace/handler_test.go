package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockStore struct {
	links map[string]*Link
	mu    sync.Mutex
}

func newMockStore() *mockStore {
	return &mockStore{
		links: make(map[string]*Link),
	}
}

func (m *mockStore) Create(urlStr, alias string) (*Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if alias != "" {
		if _, exists := m.links[alias]; exists {
			return nil, fmt.Errorf("alias already taken")
		}
	} else {
		alias = "generated"
	}

	link := &Link{
		Code:      alias,
		URL:       urlStr,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}
	m.links[alias] = link
	return link, nil
}

func (m *mockStore) Get(code string) (*Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[code]
	if !exists {
		return nil, fmt.Errorf("link not found")
	}
	return link, nil
}

func (m *mockStore) Visit(code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	link, exists := m.links[code]
	if !exists {
		return fmt.Errorf("link not found")
	}
	link.Visits++
	return nil
}

func (m *mockStore) Delete(code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.links[code]; !exists {
		return fmt.Errorf("link not found")
	}
	delete(m.links, code)
	return nil
}

func (m *mockStore) List() ([]Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	links := make([]Link, 0, len(m.links))
	for _, link := range m.links {
		links = append(links, *link)
	}
	return links, nil
}

func TestHandler_CreateLink(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "valid request",
			body:       `{"url": "https://example.com", "alias": "example"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing url",
			body:       `{"alias": "example"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "url is required",
		},
		{
			name:       "invalid json",
			body:       `{invalid json}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantError != "" {
				var resp map[string]string
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				if resp["error"] != tt.wantError {
					t.Errorf("got error %q, want %q", resp["error"], tt.wantError)
				}
			}
		})
	}

	// Test duplicate alias separately
	t.Run("duplicate alias", func(t *testing.T) {
		// Create first link
		req := httptest.NewRequest(http.MethodPost, "/api/links",
			strings.NewReader(`{"url": "https://example.com", "alias": "example"}`))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// Try to create duplicate
		req = httptest.NewRequest(http.MethodPost, "/api/links",
			strings.NewReader(`{"url": "https://another.com", "alias": "example"}`))
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusConflict {
			t.Errorf("got status %d, want %d", rr.Code, http.StatusConflict)
		}
	})
}

func TestHandler_Redirect(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	// Create a link first
	store.links["test"] = &Link{
		Code: "test",
		URL:  "https://example.com",
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusFound)
	}
	if location := rr.Header().Get("Location"); location != "https://example.com" {
		t.Errorf("got location %q, want %q", location, "https://example.com")
	}

	// Test non-existent link
	req = httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d for non-existent link, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandler_ListLinks_Unauthorized(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandler_ListLinks_Authorized(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	// Add some links
	store.links["link1"] = &Link{Code: "link1", URL: "https://example.com"}
	store.links["link2"] = &Link{Code: "link2", URL: "https://google.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	var links []Link
	if err := json.Unmarshal(rr.Body.Bytes(), &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Errorf("got %d links, want 2", len(links))
	}
}

func TestHandler_DeleteLink(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	// Add a link
	store.links["test"] = &Link{Code: "test", URL: "https://example.com"}

	// Test unauthorized
	req := httptest.NewRequest(http.MethodDelete, "/api/links/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// Test authorized delete
	req = httptest.NewRequest(http.MethodDelete, "/api/links/test", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusNoContent)
	}
	if _, exists := store.links["test"]; exists {
		t.Error("link should have been deleted")
	}

	// Test delete non-existent
	req = httptest.NewRequest(http.MethodDelete, "/api/links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	// Test unauthorized GET on /api/links/test (should be 401, not 405)
	req := httptest.NewRequest(http.MethodGet, "/api/links/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHandler_AdminAuth(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	tests := []struct {
		name        string
		authHeader  string
		wantAllowed bool
	}{
		{"no auth", "", false},
		{"wrong prefix", "Token admin-token", false},
		{"wrong token", "Bearer wrong-token", false},
		{"correct auth", "Bearer admin-token", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.wantAllowed {
				if rr.Code != http.StatusOK {
					t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
				}
			} else {
				if rr.Code != http.StatusUnauthorized {
					t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
				}
			}
		})
	}
}

func TestHandler_LargeBody(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	// Create body larger than 1 MiB
	largeBody := make([]byte, 2*1024*1024) // 2 MiB
	for i := range largeBody {
		largeBody[i] = 'x'
	}

	req := httptest.NewRequest(http.MethodPost, "/api/links", bytes.NewReader(largeBody))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestHandler_InvalidPath(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/", http.StatusNotFound},
		{http.MethodPost, "/invalid", http.StatusNotFound},
		{http.MethodGet, "/api/invalid", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tt.want {
				t.Errorf("%s %s: got status %d, want %d", tt.method, tt.path, rr.Code, tt.want)
			}
		})
	}
}

func TestHandler_VisitIncrementsCount(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	// Create link directly in store
	store.links["test"] = &Link{
		Code:   "test",
		URL:    "https://example.com",
		Visits: 0,
	}

	// Visit the link
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusFound)
	}

	link, exists := store.links["test"]
	if !exists {
		t.Fatal("link not found")
	}
	if link.Visits != 1 {
		t.Errorf("got %d visits, want 1", link.Visits)
	}
}

func TestHandler_ConcurrentAccess(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	// Set up a link
	store.links["test"] = &Link{
		Code:   "test",
		URL:    "https://example.com",
		Visits: 0,
	}

	// Run concurrent requests
	const numRequests = 100
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			done <- true
		}()
	}

	for i := 0; i < numRequests; i++ {
		<-done
	}

	link, exists := store.links["test"]
	if !exists {
		t.Fatal("link not found")
	}
	if link.Visits != numRequests {
		t.Errorf("got %d visits, want %d", link.Visits, numRequests)
	}
}

func TestHandler_JSONResponse(t *testing.T) {
	store := newMockStore()
	handler := NewHandler(store, "admin-token")

	req := httptest.NewRequest(http.MethodPost, "/api/links", 
		strings.NewReader(`{"url": "https://example.com", "alias": "example"}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("got content type %q, want application/json", contentType)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if _, ok := resp["code"]; !ok {
		t.Error("response missing 'code' field")
	}
	if _, ok := resp["url"]; !ok {
		t.Error("response missing 'url' field")
	}
	if _, ok := resp["created_at"]; !ok {
		t.Error("response missing 'created_at' field")
	}
	if _, ok := resp["visits"]; !ok {
		t.Error("response missing 'visits' field")
	}
}