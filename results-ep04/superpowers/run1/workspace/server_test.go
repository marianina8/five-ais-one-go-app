package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createTestServer(t *testing.T, adminToken string) (*httptest.Server, *FileStore) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")
	
	store, err := newFileStore(dataFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create handler with test store
	handler := newHandler(store, adminToken)
	
	server := httptest.NewServer(handler)
	
	// Add cleanup
	t.Cleanup(func() {
		server.Close()
		store.Close()
	})
	
	return server, store
}

func TestCreateLink(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	// Test valid link creation
	reqBody := `{"url": "https://example.com/page", "alias": "my-page"}`
	req, err := http.NewRequest("POST", server.URL+"/api/links", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["code"] != "my-page" {
		t.Errorf("expected code 'my-page', got %v", result["code"])
	}
	if result["url"] != "https://example.com/page" {
		t.Errorf("expected URL 'https://example.com/page', got %v", result["url"])
	}
}

func TestCreateLinkWithoutAlias(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	reqBody := `{"url": "https://example.com/page"}`
	req, err := http.NewRequest("POST", server.URL+"/api/links", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	code, ok := result["code"].(string)
	if !ok {
		t.Fatal("code is not a string")
	}
	if len(code) != 7 {
		t.Errorf("expected 7-character code, got %q (length %d)", code, len(code))
	}
}

func TestCreateLinkInvalidURL(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	testCases := []struct {
		name string
		body string
	}{
		{"not a URL", `{"url": "not-a-url"}`},
		{"no host", `{"url": "http://"}`},
		{"wrong scheme", `{"url": "ftp://example.com"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", server.URL+"/api/links", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestCreateLinkInvalidAlias(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	testCases := []struct {
		name string
		body string
	}{
		{"too short", `{"url": "https://example.com", "alias": "ab"}`},
		{"too long", `{"url": "https://example.com", "alias": "this-alias-is-way-too-long-exceeds-32-chars"}`},
		{"invalid chars", `{"url": "https://example.com", "alias": "invalid@alias"}`},
		{"reserved word", `{"url": "https://example.com", "alias": "api"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", server.URL+"/api/links", strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestFollowLink(t *testing.T) {
	server, store := createTestServer(t, "secret")
	defer server.Close()

	// First create a link
	link := Link{
		Code:      "abc123",
		URL:       "https://example.com/target",
		CreatedAt: time.Now(),
		Visits:    0,
	}
	if err := store.Create(link); err != nil {
		t.Fatal(err)
	}

	// Follow the link - create a client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	
	req, err := http.NewRequest("GET", server.URL+"/abc123", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected status 302, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "https://example.com/target" {
		t.Errorf("expected Location 'https://example.com/target', got %q", location)
	}

	// Check that visits were incremented
	storedLink, err := store.Get("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if storedLink.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", storedLink.Visits)
	}
}

func TestFollowLinkNotFound(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL+"/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestListLinksAdmin(t *testing.T) {
	server, store := createTestServer(t, "secret")
	defer server.Close()

	// Create some links
	links := []Link{
		{Code: "first", URL: "https://example.com/1", CreatedAt: time.Now().Add(-time.Hour), Visits: 5},
		{Code: "second", URL: "https://example.com/2", CreatedAt: time.Now(), Visits: 3},
	}
	for _, link := range links {
		if err := store.Create(link); err != nil {
			t.Fatal(err)
		}
	}

	// List links with admin token
	req, err := http.NewRequest("GET", server.URL+"/api/links", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 links, got %d", len(result))
	}

	// Should be sorted oldest first
	if result[0]["code"] != "first" {
		t.Errorf("expected first link 'first', got %v", result[0]["code"])
	}
	if result[1]["code"] != "second" {
		t.Errorf("expected second link 'second', got %v", result[1]["code"])
	}
}

func TestListLinksUnauthorized(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL+"/api/links", nil)
	if err != nil {
		t.Fatal(err)
	}
	// No Authorization header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

func TestDeleteLinkAdmin(t *testing.T) {
	server, store := createTestServer(t, "secret")
	defer server.Close()

	// Create a link
	link := Link{
		Code:      "todelete",
		URL:       "https://example.com",
		CreatedAt: time.Now(),
		Visits:    0,
	}
	if err := store.Create(link); err != nil {
		t.Fatal(err)
	}

	// Delete the link
	req, err := http.NewRequest("DELETE", server.URL+"/api/links/todelete", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}

	// Verify link is gone
	_, err = store.Get("todelete")
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestDeleteLinkNotFound(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	req, err := http.NewRequest("DELETE", server.URL+"/api/links/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

func TestDeleteLinkUnauthorized(t *testing.T) {
	server, _ := createTestServer(t, "secret")
	defer server.Close()

	req, err := http.NewRequest("DELETE", server.URL+"/api/links/somecode", nil)
	if err != nil {
		t.Fatal(err)
	}
	// No Authorization header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}