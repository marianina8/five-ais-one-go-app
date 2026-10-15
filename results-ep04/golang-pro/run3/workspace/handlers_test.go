package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestServer_CreateLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	// Test successful creation
	reqBody := `{"url": "https://example.com", "alias": "test"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateLink(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var link Link
	if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
		t.Fatal(err)
	}

	if link.Code != "test" {
		t.Errorf("Expected code 'test', got %s", link.Code)
	}
	if link.URL != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got %s", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("Expected visits 0, got %d", link.Visits)
	}

	// Test duplicate alias
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handleCreateLink(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", resp.StatusCode)
	}

	// Test invalid URL
	reqBody = `{"url": "not-a-url"}`
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handleCreateLink(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	// Test invalid JSON
	reqBody = `{"url": "https://example.com", invalid json`
	req = httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handleCreateLink(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestServer_Redirect(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	// Create a link first
	_, err = store.CreateLink("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Test redirect
	req := httptest.NewRequest("GET", "/test", nil)
	// Manually set path value for testing
	req.SetPathValue("code", "test")
	w := httptest.NewRecorder()

	server.handleRedirect(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected status 302, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "https://example.com" {
		t.Errorf("Expected Location header 'https://example.com', got %s", location)
	}

	// Check visits were incremented
	updatedLink, err := store.GetLink("test")
	if err != nil {
		t.Fatal(err)
	}
	if updatedLink.Visits != 1 {
		t.Errorf("Expected visits 1, got %d", updatedLink.Visits)
	}

	// Test non-existent link
	req = httptest.NewRequest("GET", "/nonexistent", nil)
	req.SetPathValue("code", "nonexistent")
	w = httptest.NewRecorder()

	server.handleRedirect(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	// Test wrong method
	req = httptest.NewRequest("POST", "/test", nil)
	req.SetPathValue("code", "test")
	w = httptest.NewRecorder()

	server.handleRedirect(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestServer_ListLinks(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	// Create some links
	links := []struct {
		url   string
		alias string
	}{
		{"https://example.com/1", "link1"},
		{"https://example.com/2", "link2"},
	}

	for _, l := range links {
		_, err := store.CreateLink(l.url, l.alias)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Test without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()

	server.handleListLinks(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Test with auth
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleListLinks(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result []Link
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 links, got %d", len(result))
	}

	// Test wrong method
	req = httptest.NewRequest("POST", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleListLinks(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestServer_DeleteLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	// Create a link first
	_, err = store.CreateLink("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Test without auth
	req := httptest.NewRequest("DELETE", "/api/links/test", nil)
	req.SetPathValue("code", "test")
	w := httptest.NewRecorder()

	server.handleDeleteLink(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Test with auth - successful deletion
	req = httptest.NewRequest("DELETE", "/api/links/test", nil)
	req.SetPathValue("code", "test")
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleDeleteLink(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}

	// Verify link is deleted
	_, err = store.GetLink("test")
	if err == nil {
		t.Error("Expected error after deletion")
	}

	// Test delete non-existent link
	req = httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
	req.SetPathValue("code", "nonexistent")
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleDeleteLink(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", resp.StatusCode)
	}

	// Test wrong method
	req = httptest.NewRequest("GET", "/api/links/test", nil)
	req.SetPathValue("code", "test")
	req.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()

	server.handleDeleteLink(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

func TestServer_AdminAuth(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	// Test without Authorization header
	req := httptest.NewRequest("GET", "/api/links", nil)
	w := httptest.NewRecorder()
	server.handleListLinks(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Test with wrong token
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	server.handleListLinks(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Test with malformed Authorization header
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Basic test-token")
	w = httptest.NewRecorder()
	server.handleListLinks(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}

	// Test missing space
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearertest-token")
	w = httptest.NewRecorder()
	server.handleListLinks(w, req)

	resp = w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", resp.StatusCode)
	}
}

func TestServer_CreateLink_AutoGeneratedCode(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	// Test successful creation without alias (auto-generated code)
	reqBody := `{"url": "https://example.com"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateLink(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}

	var link Link
	if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
		t.Fatal(err)
	}

	if link.Code == "" {
		t.Error("Expected code to be generated")
	}
	if len(link.Code) != 7 {
		t.Errorf("Expected code length 7, got %d", len(link.Code))
	}
}

func TestServer_ErrorResponses(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	testCases := []struct {
		name       string
		method     string
		path       string
		auth       bool
		body       string
		wantStatus int
	}{
		{
			name:       "POST with unknown field",
			method:     "POST",
			path:       "/api/links",
			auth:       false,
			body:       `{"url": "https://example.com", "unknown": "field"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET with empty code",
			method:     "GET",
			path:       "/",
			auth:       false,
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "DELETE with empty code",
			method:     "DELETE",
			path:       "/api/links/",
			auth:       true,
			body:       "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.auth {
				req.Header.Set("Authorization", "Bearer test-token")
			}
			if tc.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}
			
			// Set path values for tests that need them
			if tc.method == "GET" && tc.path == "/" {
				req.SetPathValue("code", "")
			} else if tc.method == "DELETE" && tc.path == "/api/links/" {
				req.SetPathValue("code", "")
			}
			
			w := httptest.NewRecorder()

			switch tc.path {
			case "/api/links":
				if tc.method == "POST" {
					server.handleCreateLink(w, req)
				} else if tc.method == "GET" {
					server.handleListLinks(w, req)
				}
			case "/":
				server.handleRedirect(w, req)
			case "/api/links/":
				server.handleDeleteLink(w, req)
			}

			resp := w.Result()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("Expected status %d, got %d", tc.wantStatus, resp.StatusCode)
			}
		})
	}
}