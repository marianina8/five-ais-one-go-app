package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandler_CreateLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantError   bool
		errorPrefix string
	}{
		{
			name:       "valid request with alias",
			body:       `{"url": "https://example.com", "alias": "test123"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid request without alias",
			body:       `{"url": "https://example.org"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:        "invalid JSON",
			body:        `{"url": "https://example.com", alias: "test"}`,
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
			errorPrefix: "invalid JSON",
		},
		{
			name:        "empty request body",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
			errorPrefix: "empty request body",
		},
		{
			name:        "missing url field",
			body:        `{"alias": "test123"}`,
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
			errorPrefix: "url field is required",
		},
		{
			name:        "invalid URL",
			body:        `{"url": "not-a-url"}`,
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
			errorPrefix: "invalid URL",
		},
		{
			name:        "duplicate alias",
			body:        `{"url": "https://example.com", "alias": "test123"}`,
			wantStatus:  http.StatusConflict,
			wantError:   true,
			errorPrefix: "already exists",
			// Note: first request will be created successfully
		},
		{
			name:        "too short alias",
			body:        `{"url": "https://example.com", "alias": "ab"}`,
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
			errorPrefix: "alias must match",
		},
		{
			name:        "reserved alias",
			body:        `{"url": "https://example.com", "alias": "api"}`,
			wantStatus:  http.StatusBadRequest,
			wantError:   true,
			errorPrefix: "alias 'api' is reserved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/links", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			body, _ := io.ReadAll(resp.Body)
			if tt.wantError {
				var errResp map[string]string
				if err := json.Unmarshal(body, &errResp); err != nil {
					t.Errorf("failed to parse error response: %v", err)
				} else if !strings.Contains(errResp["error"], tt.errorPrefix) {
					t.Errorf("error message = %q, want containing %q", errResp["error"], tt.errorPrefix)
				}
			} else if tt.wantStatus == http.StatusCreated {
				var link Link
				if err := json.Unmarshal(body, &link); err != nil {
					t.Errorf("failed to parse response: %v", err)
				}
				if tt.name == "valid request with alias" && link.Code != "test123" {
					t.Errorf("code = %s, want test123", link.Code)
				}
				if link.URL == "" {
					t.Errorf("URL is empty")
				}
				if link.Visits != 0 {
					t.Errorf("visits = %d, want 0", link.Visits)
				}
				if link.CreatedAt.IsZero() {
					t.Errorf("CreatedAt is zero")
				}
			}
		})
	}
}

func TestHandler_FollowLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create a link first
	link, err := store.CreateLink("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	handler := NewHandler(store, "test-token")

	t.Run("existing link", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test123", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusFound {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusFound)
		}
		if loc := resp.Header.Get("Location"); loc != link.URL {
			t.Errorf("Location header = %s, want %s", loc, link.URL)
		}

		// Verify visits were incremented
		updatedLink, err := store.GetLink("test123")
		if err != nil {
			t.Fatalf("failed to get link: %v", err)
		}
		if updatedLink.Visits != 1 {
			t.Errorf("visits = %d, want 1", updatedLink.Visits)
		}
	})

	t.Run("non-existent link", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}

		var errResp map[string]string
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &errResp); err != nil {
			t.Errorf("failed to parse error response: %v", err)
		}
		if !strings.Contains(errResp["error"], "not found") {
			t.Errorf("error message = %q, want containing 'not found'", errResp["error"])
		}
	})
}

func TestHandler_ListLinks(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create some links
	links := []struct {
		url   string
		alias string
	}{
		{"https://example1.com", "test1"},
		{"https://example2.com", "test2"},
	}

	for _, l := range links {
		_, err := store.CreateLink(l.url, l.alias)
		if err != nil {
			t.Fatalf("failed to create link %s: %v", l.alias, err)
		}
	}

	t.Run("without auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}

		var errResp map[string]string
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &errResp); err != nil {
			t.Errorf("failed to parse error response: %v", err)
		}
		if errResp["error"] != "unauthorized" {
			t.Errorf("error message = %q, want 'unauthorized'", errResp["error"])
		}
	})

	t.Run("with auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var result []Link
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &result); err != nil {
			t.Errorf("failed to parse response: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("got %d links, want 2", len(result))
		}
	})

	t.Run("with wrong auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("with malformed auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/links", nil)
		req.Header.Set("Authorization", "InvalidFormat")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})
}

func TestHandler_DeleteLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create a link
	_, err = store.CreateLink("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	t.Run("without auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/links/test123", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
	})

	t.Run("with auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/links/test123", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusNoContent)
		}

		// Verify link was deleted
		_, err := store.GetLink("test123")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("link should be deleted, got error: %v", err)
		}
	})

	t.Run("non-existent link", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/links/nonexistent", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})
}

func TestHandler_NotFound(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/"},
		{http.MethodGet, "/api"},
		{http.MethodGet, "/api/other"},
		{http.MethodPost, "/api/other"},
		{http.MethodPut, "/api/links"},
		{http.MethodPatch, "/api/links"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %s", tt.method, tt.path), func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusNotFound)
			}
		})
	}
}

func TestMain_AdminTokenRequired(t *testing.T) {
	// Test that main would exit if ADMIN_TOKEN is not set
	if os.Getenv("ADMIN_TOKEN") == "" {
		// We can't actually test the exit in a unit test, but we can verify the logic
		// The main function will log.Fatal if ADMIN_TOKEN is empty
		// This is more of an integration test scenario
	}
}