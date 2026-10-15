package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHandleCreateLink(t *testing.T) {
	defer os.Remove("test_api1.json")
	defer os.Remove("test_api1.json.tmp")

	store, err := NewStore("test_api1.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	tests := []struct {
		name       string
		request    map[string]string
		wantStatus int
		wantError  string
		setup      func(*Store) error
	}{
		{
			name: "valid request without alias",
			request: map[string]string{
				"url": "https://example.com",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "valid request with alias",
			request: map[string]string{
				"url":   "https://example.com",
				"alias": "my-test-alias",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "invalid URL - missing scheme",
			request: map[string]string{
				"url": "example.com",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid URL",
		},
		{
			name: "invalid URL - too long",
			request: map[string]string{
				"url": "http://example.com/" + strings.Repeat("a", 3000),
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "URL too long",
		},
		{
			name: "invalid alias - too short",
			request: map[string]string{
				"url":   "https://example.com",
				"alias": "ab",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid alias",
		},
		{
			name: "invalid alias - forbidden name",
			request: map[string]string{
				"url":   "https://example.com",
				"alias": "api",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid alias",
		},
		{
			name: "duplicate alias",
			request: map[string]string{
				"url":   "https://example.com/other",
				"alias": "my-test-alias",
			},
			wantStatus: http.StatusConflict,
			wantError:  "alias already taken",
			setup: func(s *Store) error {
				link := Link{
					Code:      "my-test-alias",
					URL:       "https://example.com",
					CreatedAt: time.Now().UTC(),
					Visits:    0,
				}
				return s.Create(link)
			},
		},
		{
			name: "invalid JSON",
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh store for each test
			store, err := NewStore(fmt.Sprintf("test_api1_%s.json", tt.name))
			if err != nil {
				t.Fatalf("NewStore failed: %v", err)
			}
			defer os.Remove(fmt.Sprintf("test_api1_%s.json", tt.name))
			defer os.Remove(fmt.Sprintf("test_api1_%s.json.tmp", tt.name))
			defer store.Close()

			// Run setup if defined
			if tt.setup != nil {
				if err := tt.setup(store); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			var reqBody []byte
			if tt.name == "invalid JSON" {
				reqBody = []byte("{invalid json")
			} else {
				reqBody, _ = json.Marshal(tt.request)
			}

			req := httptest.NewRequest("POST", "/api/links", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()

			store.handleCreateLink(resp, req)

			if resp.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.Code, tt.wantStatus)
			}

			if tt.wantError != "" {
				var result map[string]string
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if !strings.Contains(strings.ToLower(result["error"]), strings.ToLower(tt.wantError)) {
					t.Errorf("got error %q, want error containing %q", result["error"], tt.wantError)
				}
			} else if tt.wantStatus == http.StatusCreated {
				var link Link
				if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if tt.request != nil && link.URL != tt.request["url"] {
					t.Errorf("got URL %q, want %q", link.URL, tt.request["url"])
				}
				if tt.request != nil && tt.request["alias"] != "" && link.Code != tt.request["alias"] {
					t.Errorf("got code %q, want %q", link.Code, tt.request["alias"])
				}
				if tt.request != nil && tt.request["alias"] == "" && len(link.Code) != 7 {
					t.Errorf("got code length %d, want 7", len(link.Code))
				}
				if link.Visits != 0 {
					t.Errorf("got visits %d, want 0", link.Visits)
				}
			}
		})
	}
}

func TestHandleRedirect(t *testing.T) {
	defer os.Remove("test_api2.json")
	defer os.Remove("test_api2.json.tmp")

	store, err := NewStore("test_api2.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create a link first
	link := Link{
		Code:      "test123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}
	if err := store.Create(link); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create a test server with the actual router
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{code}", store.handleRedirect)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Create HTTP client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test successful redirect
	resp, err := client.Get(ts.URL + "/test123")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if location := resp.Header.Get("Location"); location != "https://example.com" {
		t.Errorf("got Location %q, want %q", location, "https://example.com")
	}

	// Verify visits were incremented
	updatedLink, exists := store.Get("test123")
	if !exists {
		t.Fatal("link should still exist")
	}
	if updatedLink.Visits != 1 {
		t.Errorf("got visits %d, want 1", updatedLink.Visits)
	}

	// Test non-existent code
	resp, err = client.Get(ts.URL + "/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandleListLinks(t *testing.T) {
	defer os.Remove("test_api3.json")
	defer os.Remove("test_api3.json.tmp")

	store, err := NewStore("test_api3.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create some links
	links := []Link{
		{
			Code:      "link1",
			URL:       "https://example.com/1",
			CreatedAt: time.Now().UTC().Add(-time.Hour),
			Visits:    5,
		},
		{
			Code:      "link2",
			URL:       "https://example.com/2",
			CreatedAt: time.Now().UTC(),
			Visits:    10,
		},
	}

	for _, link := range links {
		if err := store.Create(link); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Create a test server with auth middleware
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/links", store.authorizeAdmin("test-token", store.handleListLinks))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Create HTTP client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test without auth
	resp, err := client.Get(ts.URL + "/api/links")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d without auth, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Test with auth
	req, _ := http.NewRequest("GET", ts.URL+"/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d with auth, want %d", resp.StatusCode, http.StatusOK)
	}

	var result []Link
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("got %d links, want 2", len(result))
	}

	// Should be sorted by creation time (oldest first)
	if result[0].Code != "link1" {
		t.Errorf("got first link %q, want %q (oldest first)", result[0].Code, "link1")
	}
	if result[1].Code != "link2" {
		t.Errorf("got second link %q, want %q (newest second)", result[1].Code, "link2")
	}
}

func TestHandleDeleteLink(t *testing.T) {
	defer os.Remove("test_api4.json")
	defer os.Remove("test_api4.json.tmp")

	store, err := NewStore("test_api4.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create a link first
	link := Link{
		Code:      "test123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}
	if err := store.Create(link); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create a test server with auth middleware
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/links/{code}", store.authorizeAdmin("test-token", store.handleDeleteLink))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Create HTTP client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Test without auth
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/links/test123", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got status %d without auth, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// Test with auth - successful delete
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/links/test123", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// Verify link is deleted
	_, exists := store.Get("test123")
	if exists {
		t.Error("link should be deleted")
	}

	// Test delete non-existent link with auth
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/links/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("got status %d for non-existent link, want %d", resp.StatusCode, http.StatusNotFound)
	}
}