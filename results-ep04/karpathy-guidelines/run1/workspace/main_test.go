package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Set up test environment
	os.Setenv("ADMIN_TOKEN", "test-token")
	
	code := m.Run()
	
	// Clean up
	os.Unsetenv("ADMIN_TOKEN")
	os.Exit(code)
}

func TestServer_CreateLink(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	
	server := httptest.NewServer(NewServer(store, "test-token"))
	defer server.Close()

	tests := []struct {
		name       string
		reqBody    string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "valid request without alias",
			reqBody:    `{"url": "https://example.com"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid request with alias",
			reqBody:    `{"url": "https://example.com", "alias": "test123"}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid URL",
			reqBody:    `{"url": "not-a-url"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name:       "missing URL",
			reqBody:    `{"alias": "test"}`,
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
			reqBody:    `{"url": "https://example.com", "alias": "test@123"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", server.URL+"/api/links", bytes.NewBufferString(tt.reqBody))
			req.Header.Set("Content-Type", "application/json")
			
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantError {
				var errResp map[string]string
				if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
					t.Fatal(err)
				}
				if _, ok := errResp["error"]; !ok {
					t.Errorf("expected error response")
				}
			} else if resp.StatusCode == http.StatusCreated {
				var link Link
				if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
					t.Fatal(err)
				}
				if link.URL != "https://example.com" {
					t.Errorf("got URL %s, want https://example.com", link.URL)
				}
				if link.Visits != 0 {
					t.Errorf("got visits %d, want 0", link.Visits)
				}
			}
		})
	}
}

func TestServer_Redirect(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	
	server := httptest.NewServer(NewServer(store, "test-token"))
	defer server.Close()

	// First create a link
	reqBody := `{"url": "https://example.com", "alias": "test-redirect"}`
	req, _ := http.NewRequest("POST", server.URL+"/api/links", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Test redirect without following
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	
	req, _ = http.NewRequest("GET", server.URL+"/test-redirect", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if location := resp.Header.Get("Location"); location != "https://example.com" {
		t.Errorf("got location %s, want https://example.com", location)
	}
}

func TestServer_ListLinks(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	
	server := httptest.NewServer(NewServer(store, "test-token"))
	defer server.Close()

	// Create test links
	store.Create("https://example.com/1", "link1")
	store.Create("https://example.com/2", "link2")

	// Test without auth
	req, _ := http.NewRequest("GET", server.URL+"/api/links", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d without auth, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Test with auth
	req, _ = http.NewRequest("GET", server.URL+"/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d with auth, want %d", resp.StatusCode, http.StatusOK)
	}

	var links []Link
	if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
		t.Fatal(err)
	}

	if len(links) != 2 {
		t.Errorf("got %d links, want 2", len(links))
	}
}

func TestServer_DeleteLink(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	
	server := httptest.NewServer(NewServer(store, "test-token"))
	defer server.Close()

	// Create test link
	store.Create("https://example.com", "to-delete")

	// Test delete without auth
	req, _ := http.NewRequest("DELETE", server.URL+"/api/links/to-delete", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d without auth, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Test delete with auth
	req, _ = http.NewRequest("DELETE", server.URL+"/api/links/to-delete", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("got status %d with auth, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// Verify link is gone
	_, err = store.Get("to-delete")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("link should be deleted")
	}
}

func createTestStore(t *testing.T) (Store, func()) {
	t.Helper()
	
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return store, cleanup
}