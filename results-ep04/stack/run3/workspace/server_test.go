package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateLinkHandler(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := fmt.Sprintf("%s/test.json", tmpDir)
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(":0", store, "admin-token")

	tests := []struct {
		name         string
		requestBody  string
		wantStatus   int
		wantContains string
	}{
		{
			name:        "valid link with alias",
			requestBody: `{"url": "https://example.com", "alias": "my-page"}`,
			wantStatus:  201,
		},
		{
			name:        "valid link without alias",
			requestBody: `{"url": "https://example.com"}`,
			wantStatus:  201,
		},
		{
			name:         "invalid URL scheme",
			requestBody:  `{"url": "ftp://example.com"}`,
			wantStatus:   400,
			wantContains: "URL must be http or https",
		},
		{
			name:         "URL too long",
			requestBody:  fmt.Sprintf(`{"url": "https://example.com/%s"}`, strings.Repeat("a", 2048)),
			wantStatus:   400,
			wantContains: "url too long",
		},
		{
			name:         "invalid alias length",
			requestBody:  `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus:   400,
			wantContains: "alias must match",
		},
		{
			name:         "reserved alias",
			requestBody:  `{"url": "https://example.com", "alias": "api"}`,
			wantStatus:   400,
			wantContains: "'api' is a reserved alias",
		},
		{
			name:         "duplicate alias",
			requestBody:  `{"url": "https://example.com", "alias": "my-page"}`,
			wantStatus:   409,
			wantContains: "already exists",
		},
		{
			name:         "invalid JSON",
			requestBody:  `{invalid json}`,
			wantStatus:   400,
			wantContains: "invalid JSON",
		},
		{
			name:         "missing url field",
			requestBody:  `{"alias": "my-page"}`,
			wantStatus:   400,
			wantContains: "url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			server.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("HTTP status = %d, want %d", rr.Code, tt.wantStatus)
			}

			if tt.wantContains != "" {
				body := rr.Body.String()
				if !strings.Contains(body, tt.wantContains) {
					t.Errorf("Response body %q does not contain %q", body, tt.wantContains)
				}
			}

			if rr.Code == 201 {
				var resp map[string]interface{}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Errorf("Failed to parse response JSON: %v", err)
				}

				// Check response fields
				if code, ok := resp["code"].(string); !ok || code == "" {
					t.Error("Response missing or empty 'code' field")
				}
				if url, ok := resp["url"].(string); !ok || url != "https://example.com" {
					t.Errorf("Response 'url' field incorrect: %v", resp["url"])
				}
				if created, ok := resp["created_at"].(string); !ok || created == "" {
					t.Error("Response missing or empty 'created_at' field")
				}
				if visits, ok := resp["visits"].(float64); !ok || visits != 0 {
					t.Error("Response 'visits' field should be 0")
				}
			}
		})
	}
}

func TestFollowLinkHandler(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := fmt.Sprintf("%s/test.json", tmpDir)
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Add a test link
	link := &Link{
		Code:      "test123",
		URL:       "https://example.com/path",
		CreatedAt: time.Now(),
		Visits:    0,
	}
	if err := store.Add(link); err != nil {
		t.Fatalf("Failed to add test link: %v", err)
	}

	server := NewServer(":0", store, "admin-token")

	t.Run("existing link", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test123", nil)
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusFound)
		}

		location := rr.Header().Get("Location")
		if location != "https://example.com/path" {
			t.Errorf("Location header = %q, want %q", location, "https://example.com/path")
		}

		// Verify visit count incremented
		updatedLink, err := store.Get("test123")
		if err != nil {
			t.Fatalf("Failed to get updated link: %v", err)
		}
		if updatedLink.Visits != 1 {
			t.Errorf("Visit count = %d, want 1", updatedLink.Visits)
		}
	})

	t.Run("non-existent link", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/nonexistent", nil)
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusNotFound)
		}

		var resp map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse response JSON: %v", err)
		}
		if resp["error"] == "" {
			t.Error("Response missing error message")
		}
	})
}

func TestListLinksHandler(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := fmt.Sprintf("%s/test.json", tmpDir)
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(":0", store, "admin-token")

	// Add test links
	links := []*Link{
		{
			Code:      "first",
			URL:       "https://example.com/first",
			CreatedAt: time.Now().Add(-2 * time.Hour),
			Visits:    5,
		},
		{
			Code:      "second",
			URL:       "https://example.com/second",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			Visits:    10,
		},
	}

	for _, link := range links {
		if err := store.Add(link); err != nil {
			t.Fatalf("Failed to add test link: %v", err)
		}
	}

	t.Run("with admin auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/links", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
		}

		var resp []map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse response JSON: %v", err)
		}

		if len(resp) != 2 {
			t.Errorf("Response contains %d links, want 2", len(resp))
		}

		// Should be sorted by CreatedAt (oldest first)
		if resp[0]["code"].(string) != "first" || resp[1]["code"].(string) != "second" {
			t.Error("Links not sorted by creation time (oldest first)")
		}
	})

	t.Run("without admin auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/links", nil)
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("with wrong admin auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/links", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusUnauthorized)
		}
	})
}

func TestDeleteLinkHandler(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := fmt.Sprintf("%s/test.json", tmpDir)
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(":0", store, "admin-token")

	// Add a test link
	link := &Link{
		Code:      "todelete",
		URL:       "https://example.com",
		CreatedAt: time.Now(),
		Visits:    0,
	}
	if err := store.Add(link); err != nil {
		t.Fatalf("Failed to add test link: %v", err)
	}

	t.Run("delete with admin auth", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/links/todelete", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusNoContent)
		}

		// Verify link is deleted
		if _, err := store.Get("todelete"); err == nil {
			t.Error("Link should be deleted but still exists")
		}
	})

	t.Run("delete non-existent link", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})

	t.Run("delete without admin auth", func(t *testing.T) {
		// Add a new link first
		link2 := &Link{
			Code:      "another",
			URL:       "https://example.com",
			CreatedAt: time.Now(),
			Visits:    0,
		}
		if err := store.Add(link2); err != nil {
			t.Fatalf("Failed to add test link: %v", err)
		}

		req := httptest.NewRequest("DELETE", "/api/links/another", nil)
		rr := httptest.NewRecorder()

		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusUnauthorized)
		}
	})
}

func TestMethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := fmt.Sprintf("%s/test.json", tmpDir)
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(":0", store, "admin-token")

	// Test PUT on /api/links (only POST allowed)
	req := httptest.NewRequest("PUT", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestLargeRequestBody(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := fmt.Sprintf("%s/test.json", tmpDir)
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(":0", store, "admin-token")

	// Create a request body > 1 MiB
	largeBody := strings.Repeat("x", 1024*1024+1) // 1 MiB + 1 byte
	req := httptest.NewRequest("POST", "/api/links", bytes.NewBufferString(largeBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("HTTP status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}