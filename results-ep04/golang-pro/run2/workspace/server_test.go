package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestServer(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.json")

	// Setup for tests
	adminToken := "test-admin-token"

	t.Run("Root endpoint", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", rr.Code)
		}

		expected := "URL Shortener Service\n"
		if body := rr.Body.String(); body != expected {
			t.Errorf("Expected body %q, got %q", expected, body)
		}
	})

	t.Run("Create link - success", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		reqBody := `{"url": "https://example.com", "alias": "test-alias"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if code, ok := response["code"].(string); !ok || code != "test-alias" {
			t.Errorf("Expected code 'test-alias', got %v", response["code"])
		}

		if url, ok := response["url"].(string); !ok || url != "https://example.com" {
			t.Errorf("Expected URL 'https://example.com', got %v", response["url"])
		}

		if visits, ok := response["visits"].(float64); !ok || visits != 0 {
			t.Errorf("Expected 0 visits, got %v", response["visits"])
		}
	})

	t.Run("Create link - missing URL", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		reqBody := `{"alias": "test-alias"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Create link - invalid URL", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		reqBody := `{"url": "invalid-url", "alias": "test-alias"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Create link - duplicate alias", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		// First request
		reqBody := `{"url": "https://example.com", "alias": "duplicate"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("First create failed with status %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Second request with same alias
		req2 := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req2.Header.Set("Content-Type", "application/json")
		rr2 := httptest.NewRecorder()
		server.ServeHTTP(rr2, req2)

		if rr2.Code != http.StatusConflict {
			t.Errorf("Expected status 409 for duplicate alias, got %d. Body: %s", rr2.Code, rr2.Body.String())
		}
	})

	t.Run("Create link - invalid alias format", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		reqBody := `{"url": "https://example.com", "alias": "ab"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid alias, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Create link - alias 'api' not allowed", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		reqBody := `{"url": "https://example.com", "alias": "api"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for alias 'api', got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Redirect link - success", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		// First create a link
		reqBody := `{"url": "https://example.com", "alias": "redirect-test"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Create link failed with status %d. Body: %s", rr.Code, rr.Body.String())
		}

		// Now redirect
		req2 := httptest.NewRequest("GET", "/redirect-test", nil)
		rr2 := httptest.NewRecorder()
		server.ServeHTTP(rr2, req2)

		if rr2.Code != http.StatusFound {
			t.Errorf("Expected status 302, got %d", rr2.Code)
		}

		location := rr2.Header().Get("Location")
		if location != "https://example.com" {
			t.Errorf("Expected Location 'https://example.com', got %q", location)
		}
	})

	t.Run("Redirect link - not found", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		req := httptest.NewRequest("GET", "/nonexistent", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("List links - unauthorized", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		req := httptest.NewRequest("GET", "/api/links", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 without auth, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("List links - authorized", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		// Create a link first
		reqBody := `{"url": "https://example.com", "alias": "list-test"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		// Now list
		req2 := httptest.NewRequest("GET", "/api/links", nil)
		req2.Header.Set("Authorization", "Bearer "+adminToken)
		rr2 := httptest.NewRecorder()
		server.ServeHTTP(rr2, req2)

		if rr2.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", rr2.Code, rr2.Body.String())
		}

		var links []map[string]interface{}
		if err := json.Unmarshal(rr2.Body.Bytes(), &links); err != nil {
			t.Fatalf("Failed to unmarshal links: %v", err)
		}

		if len(links) < 1 {
			t.Fatalf("Expected at least 1 link, got %d", len(links))
		}

		found := false
		for _, link := range links {
			if code, ok := link["code"].(string); ok && code == "list-test" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Created link not found in list")
		}
	})

	t.Run("Delete link - unauthorized", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		req := httptest.NewRequest("DELETE", "/api/links/some-code", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 without auth, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Delete link - authorized", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		// Create a link first
		reqBody := `{"url": "https://example.com", "alias": "delete-test"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		// Now delete
		req2 := httptest.NewRequest("DELETE", "/api/links/delete-test", nil)
		req2.Header.Set("Authorization", "Bearer "+adminToken)
		rr2 := httptest.NewRecorder()
		server.ServeHTTP(rr2, req2)

		if rr2.Code != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d. Body: %s", rr2.Code, rr2.Body.String())
		}

		// Verify it's gone
		req3 := httptest.NewRequest("GET", "/delete-test", nil)
		rr3 := httptest.NewRecorder()
		server.ServeHTTP(rr3, req3)

		if rr3.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 after delete, got %d. Body: %s", rr3.Code, rr3.Body.String())
		}
	})

	t.Run("Delete link - not found", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		req := httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 for non-existent link, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Method not allowed", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		// PUT on /api/links
		req := httptest.NewRequest("PUT", "/api/links", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405 for PUT on /api/links, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Visit counting", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		server := NewServer(store, adminToken)

		// Create a link
		reqBody := `{"url": "https://example.com", "alias": "visit-count"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		// Redirect 3 times (incrementing visits each time)
		for i := 0; i < 3; i++ {
			req2 := httptest.NewRequest("GET", "/visit-count", nil)
			rr2 := httptest.NewRecorder()
			server.ServeHTTP(rr2, req2)

			if rr2.Code != http.StatusFound {
				t.Errorf("Redirect %d failed with status %d", i+1, rr2.Code)
			}
		}

		// List links and check visit count
		req3 := httptest.NewRequest("GET", "/api/links", nil)
		req3.Header.Set("Authorization", "Bearer "+adminToken)
		rr3 := httptest.NewRecorder()
		server.ServeHTTP(rr3, req3)

		var links []map[string]interface{}
		if err := json.Unmarshal(rr3.Body.Bytes(), &links); err != nil {
			t.Fatalf("Failed to unmarshal links: %v", err)
		}

		for _, link := range links {
			if code, ok := link["code"].(string); ok && code == "visit-count" {
				if visits, ok := link["visits"].(float64); ok && visits != 3 {
					t.Errorf("Expected 3 visits, got %v", visits)
				}
				return
			}
		}

		t.Error("Could not find 'visit-count' link in list")
	})
}