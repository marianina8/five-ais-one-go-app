package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestIntegrationExample(t *testing.T) {
	// Create a temporary data file
	tempFile, err := os.CreateTemp("", "integration-example-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Start server with test token
	os.Setenv("ADMIN_TOKEN", "test-secret-123")
	defer os.Unsetenv("ADMIN_TOKEN")

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, "test-secret-123")
	
	// Use httptest server for testing
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			server.handleRoot(w, r)
		case "/api/links":
			server.handleLinksAPI(w, r)
		default:
			if len(r.URL.Path) > len("/api/links/") && r.URL.Path[:len("/api/links/")] == "/api/links/" {
				server.handleLinkAPI(w, r)
			} else {
				server.handleRoot(w, r)
			}
		}
	}))
	defer testServer.Close()

	// Test 1: Create a link with alias
	t.Run("CreateLinkWithAlias", func(t *testing.T) {
		reqBody := `{"url": "https://google.com", "alias": "google"}`
		resp, err := http.Post(testServer.URL+"/api/links", "application/json", bytes.NewReader([]byte(reqBody)))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected status 201, got %d", resp.StatusCode)
		}

		var link Link
		if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
			t.Fatal(err)
		}

		if link.Code != "google" {
			t.Errorf("expected code 'google', got %s", link.Code)
		}
		if link.Visits != 0 {
			t.Errorf("expected 0 visits, got %d", link.Visits)
		}
	})

	// Test 2: Create a link without alias (random code)
	t.Run("CreateLinkRandomCode", func(t *testing.T) {
		reqBody := `{"url": "https://example.com"}`
		resp, err := http.Post(testServer.URL+"/api/links", "application/json", bytes.NewReader([]byte(reqBody)))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected status 201, got %d", resp.StatusCode)
		}

		var link Link
		if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
			t.Fatal(err)
		}

		if len(link.Code) != 7 {
			t.Errorf("expected code length 7, got %d", len(link.Code))
		}
	})

	// Test 3: Redirect link
	t.Run("RedirectLink", func(t *testing.T) {
		reqBody := `{"url": "https://redirect-test.com", "alias": "redirect-test"}`
		resp, err := http.Post(testServer.URL+"/api/links", "application/json", bytes.NewReader([]byte(reqBody)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		// Test redirect
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err = client.Get(testServer.URL + "/redirect-test")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusFound {
			t.Errorf("expected status 302, got %d", resp.StatusCode)
		}

		location := resp.Header.Get("Location")
		if location != "https://redirect-test.com" {
			t.Errorf("expected Location 'https://redirect-test.com', got %s", location)
		}
	})

	// Test 4: List links (admin)
	t.Run("ListLinks", func(t *testing.T) {
		req, err := http.NewRequest("GET", testServer.URL+"/api/links", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer test-secret-123")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		var links []Link
		if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
			t.Fatal(err)
		}

		if len(links) < 3 {
			t.Errorf("expected at least 3 links, got %d", len(links))
		}
	})

	// Test 5: Delete link (admin)  
	t.Run("DeleteLink", func(t *testing.T) {
		// First create a link to delete
		reqBody := `{"url": "https://delete-test.com", "alias": "delete-me"}`
		resp, err := http.Post(testServer.URL+"/api/links", "application/json", bytes.NewReader([]byte(reqBody)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		// Delete it
		req, err := http.NewRequest("DELETE", testServer.URL+"/api/links/delete-me", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer test-secret-123")

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", resp.StatusCode)
		}

		// Verify it's gone
		resp, err = http.Get(testServer.URL + "/delete-me")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected status 404 after deletion, got %d", resp.StatusCode)
		}
	})
}