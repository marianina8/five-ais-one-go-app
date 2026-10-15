package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) (*httptest.Server, *Store) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	server := NewServer(store, "test-token")
	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		store.Close()
	})

	return ts, store
}

func TestCreateLink(t *testing.T) {
	ts, _ := setupTestServer(t)

	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "valid request without alias",
			requestBody:    `{"url": "https://example.com"}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "valid request with alias",
			requestBody:    `{"url": "https://google.com", "alias": "my-google"}`,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "invalid URL",
			requestBody:    `{"url": "not-a-url"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid URL",
		},
		{
			name:           "missing URL",
			requestBody:    `{}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "url is required",
		},
		{
			name:           "empty request body",
			requestBody:    ``,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "empty request body",
		},
		{
			name:           "invalid JSON",
			requestBody:    `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid JSON",
		},
		{
			name:           "reserved alias",
			requestBody:    `{"url": "https://example.com", "alias": "api"}`,
			expectedStatus: http.StatusBadRequest,
			expectedError:  "invalid alias",
		},
		{
			name:           "URL too long",
			requestBody:    fmt.Sprintf(`{"url": "https://example.com/%s"}`, strings.Repeat("a", 2048)),
			expectedStatus: http.StatusBadRequest,
			expectedError:  "URL too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/links", strings.NewReader(tt.requestBody))
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			if tt.expectedError != "" {
				var result map[string]string
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if !strings.Contains(result["error"], tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, result["error"])
				}
			}

			if tt.expectedStatus == http.StatusCreated {
				var result map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if result["code"] == "" {
					t.Error("expected non-empty code in response")
				}
			}
		})
	}
}

func TestCreateLinkDuplicateAlias(t *testing.T) {
	ts, _ := setupTestServer(t)

	// First request should succeed
	req1, err := http.NewRequest(http.MethodPost, ts.URL+"/api/links",
		strings.NewReader(`{"url": "https://example1.com", "alias": "my-alias"}`))
	if err != nil {
		t.Fatalf("failed to create first request: %v", err)
	}
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("failed to make first request: %v", err)
	}
	resp1.Body.Close()

	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected first request to succeed with 201, got %d", resp1.StatusCode)
	}

	// Second request with same alias should fail
	req2, err := http.NewRequest(http.MethodPost, ts.URL+"/api/links",
		strings.NewReader(`{"url": "https://example2.com", "alias": "my-alias"}`))
	if err != nil {
		t.Fatalf("failed to create second request: %v", err)
	}
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("failed to make second request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected status 409 for duplicate alias, got %d", resp2.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp2.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(result["error"], "already taken") {
		t.Errorf("expected error about alias already taken, got %q", result["error"])
	}
}

func TestRedirectLink(t *testing.T) {
	ts, store := setupTestServer(t)

	// Create a link
	_, err := store.CreateLink("https://example.com", "my-link")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Test redirect
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/my-link", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected status 302, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "https://example.com" {
		t.Errorf("expected Location header to be https://example.com, got %s", resp.Header.Get("Location"))
	}

	// Verify visits were incremented
	linkAfter, err := store.GetLink("my-link")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if linkAfter.Visits != 1 {
		t.Errorf("expected 1 visit after redirect, got %d", linkAfter.Visits)
	}
}

func TestRedirectNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/nonexistent", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent link, got %d", resp.StatusCode)
	}
}

func TestListLinksUnauthorized(t *testing.T) {
	ts, _ := setupTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/links", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", resp.StatusCode)
	}
}

func TestListLinksAuthorized(t *testing.T) {
	ts, store := setupTestServer(t)

	// Create some links
	store.CreateLink("https://example1.com", "link1")
	store.CreateLink("https://example2.com", "link2")

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/links", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var links []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}
}

func TestDeleteLink(t *testing.T) {
	ts, store := setupTestServer(t)

	// Create a link
	_, err := store.CreateLink("https://example.com", "to-delete")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Try to delete without auth
	req1, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/links/to-delete", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	resp1.Body.Close()

	if resp1.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", resp1.StatusCode)
	}

	// Delete with auth
	req2, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/links/to-delete", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req2.Header.Set("Authorization", "Bearer test-token")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", resp2.StatusCode)
	}

	// Verify link is gone
	_, err = store.GetLink("to-delete")
	if err != ErrNotFound {
		t.Errorf("expected link to be deleted, got error: %v", err)
	}
}

func TestDeleteLinkNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/links/nonexistent", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent link, got %d", resp.StatusCode)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Test PUT on /api/links
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/links", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for PUT, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["error"] != "method not allowed" {
		t.Errorf("expected 'method not allowed' error, got %q", result["error"])
	}
}

func TestRequestSizeLimit(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Create a request body larger than 1 MiB
	largeBody := bytes.Repeat([]byte("a"), 2<<20) // 2 MiB

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/links", bytes.NewReader(largeBody))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for large request, got %d", resp.StatusCode)
	}
}

func TestRootHandler(t *testing.T) {
	ts, _ := setupTestServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for root, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != "URL Shortener" {
		t.Errorf("expected 'URL Shortener' in response, got %q", string(body))
	}
}