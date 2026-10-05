package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestIntegration(t *testing.T) {
	// Create a temporary data file
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/data.json"

	// Start server
	adminToken := "test-admin-token"
	os.Setenv("ADMIN_TOKEN", adminToken)

	store, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(store, adminToken)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := srv.Client()

	t.Run("Create and redirect link", func(t *testing.T) {
		// Create a link
		reqBody := `{"url": "https://example.com", "alias": "example"}`
		req, err := http.NewRequest("POST", srv.URL+"/api/links", bytes.NewBufferString(reqBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected status 201, got %d", resp.StatusCode)
		}

		var createResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			t.Fatal(err)
		}

		if createResp["code"] != "example" {
			t.Errorf("expected code 'example', got %v", createResp["code"])
		}

		// Redirect to the link
		req, err = http.NewRequest("GET", srv.URL+"/example", nil)
		if err != nil {
			t.Fatal(err)
		}

		// Don't follow redirects
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusFound {
			t.Errorf("expected status 302, got %d", resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "https://example.com" {
			t.Errorf("expected location https://example.com, got %s", location)
		}
	})

	t.Run("List links requires admin auth", func(t *testing.T) {
		// Without auth
		req, err := http.NewRequest("GET", srv.URL+"/api/links", nil)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401 without auth, got %d", resp.StatusCode)
		}

		// With auth
		req, err = http.NewRequest("GET", srv.URL+"/api/links", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200 with auth, got %d", resp.StatusCode)
		}

		var links []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
			t.Fatal(err)
		}

		if len(links) == 0 {
			t.Error("expected at least one link")
		}
	})

	t.Run("Delete link", func(t *testing.T) {
		// Create a link to delete
		reqBody := `{"url": "https://delete-me.com", "alias": "todelete"}`
		req, err := http.NewRequest("POST", srv.URL+"/api/links", bytes.NewBufferString(reqBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		// Delete without auth (should fail)
		req, err = http.NewRequest("DELETE", srv.URL+"/api/links/todelete", nil)
		if err != nil {
			t.Fatal(err)
		}

		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected status 401 without auth, got %d", resp.StatusCode)
		}

		// Delete with auth
		req, err = http.NewRequest("DELETE", srv.URL+"/api/links/todelete", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+adminToken)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", resp.StatusCode)
		}

		// Verify link is gone
		req, err = http.NewRequest("GET", srv.URL+"/todelete", nil)
		if err != nil {
			t.Fatal(err)
		}

		client.CheckRedirect = nil // Reset
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected status 404 for deleted link, got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid URL validation", func(t *testing.T) {
		testCases := []struct {
			name string
			body string
			want int
		}{
			{"no scheme", `{"url": "example.com"}`, http.StatusBadRequest},
			{"ftp scheme", `{"url": "ftp://example.com"}`, http.StatusBadRequest},
			{"no host", `{"url": "https://"}`, http.StatusBadRequest},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req, err := http.NewRequest("POST", srv.URL+"/api/links", bytes.NewBufferString(tc.body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.want {
					t.Errorf("%s: expected status %d, got %d", tc.name, tc.want, resp.StatusCode)
				}
			})
		}
	})

	t.Run("Invalid alias validation", func(t *testing.T) {
		testCases := []struct {
			name string
			body string
			want int
		}{
			{"too short", `{"url": "https://example.com", "alias": "ab"}`, http.StatusBadRequest},
			{"reserved api", `{"url": "https://example.com", "alias": "api"}`, http.StatusBadRequest},
			{"invalid chars", `{"url": "https://example.com", "alias": "test@123"}`, http.StatusBadRequest},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req, err := http.NewRequest("POST", srv.URL+"/api/links", bytes.NewBufferString(tc.body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.want {
					t.Errorf("%s: expected status %d, got %d", tc.name, tc.want, resp.StatusCode)
				}
			})
		}
	})
}

func TestPersistenceAcrossRestarts(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/persist.json"
	
	// First instance: create a link
	store1, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	
	_, err = store1.Create("https://persistent.com", "persist")
	if err != nil {
		t.Fatal(err)
	}
	
	// Close first instance
	// (in real scenario, server would restart)
	
	// Second instance: should see the same link
	store2, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	
	link2, err := store2.Get("persist")
	if err != nil {
		t.Fatal(err)
	}
	
	if link2.URL != "https://persistent.com" {
		t.Errorf("persisted URL mismatch: got %s, want https://persistent.com", link2.URL)
	}
}

func TestVisitCountPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/visits.json"
	
	store, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	
	_, err = store.Create("https://visits.com", "visits")
	if err != nil {
		t.Fatal(err)
	}
	
	// Increment visits
	for i := 0; i < 5; i++ {
		if err := store.Visit("visits"); err != nil {
			t.Fatal(err)
		}
	}
	
	// Reload store
	store2, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	
	link2, err := store2.Get("visits")
	if err != nil {
		t.Fatal(err)
	}
	
	if link2.Visits != 5 {
		t.Errorf("persisted visits count mismatch: got %d, want 5", link2.Visits)
	}
}