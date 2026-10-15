package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	// Clean up test files
	defer os.Remove("integration.json")
	defer os.Remove("integration.json.tmp")
	
	// Create store
	store, err := NewStore("integration.json")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()
	
	// Create server
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", store.handleCreateLink)
	mux.HandleFunc("GET /api/links", store.authorizeAdmin("admin-secret", store.handleListLinks))
	mux.HandleFunc("DELETE /api/links/{code}", store.authorizeAdmin("admin-secret", store.handleDeleteLink))
	mux.HandleFunc("GET /{code}", store.handleRedirect)
	mux.HandleFunc("HEAD /{code}", store.handleRedirect)
	
	ts := httptest.NewServer(mux)
	defer ts.Close()
	
	// Create HTTP client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	
	t.Run("Create link without alias", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"url": "https://example.com/test1",
		})
		
		resp, err := client.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}
		
		var link Link
		if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		
		if link.URL != "https://example.com/test1" {
			t.Errorf("Expected URL https://example.com/test1, got %s", link.URL)
		}
		if len(link.Code) != 7 {
			t.Errorf("Expected code length 7, got %d", len(link.Code))
		}
		if link.Visits != 0 {
			t.Errorf("Expected 0 visits, got %d", link.Visits)
		}
		if link.CreatedAt.IsZero() {
			t.Error("Expected CreatedAt to be set")
		}
		
		// Test redirect
		resp, err = client.Get(ts.URL + "/" + link.Code)
		if err != nil {
			t.Fatalf("Redirect request failed: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusFound {
			t.Errorf("Expected status 302, got %d", resp.StatusCode)
		}
		if location := resp.Header.Get("Location"); location != "https://example.com/test1" {
			t.Errorf("Expected Location https://example.com/test1, got %s", location)
		}
		
		// Verify visit was counted
		link2, exists := store.Get(link.Code)
		if !exists {
			t.Fatal("Link should exist")
		}
		if link2.Visits != 1 {
			t.Errorf("Expected 1 visit, got %d", link2.Visits)
		}
	})
	
	t.Run("Create link with alias", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"url":   "https://example.com/test2",
			"alias": "test-alias",
		})
		
		resp, err := client.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}
		
		var link Link
		if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		
		if link.Code != "test-alias" {
			t.Errorf("Expected code test-alias, got %s", link.Code)
		}
		
		// Test duplicate alias
		resp, err = client.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("Expected status 409 for duplicate alias, got %d", resp.StatusCode)
		}
	})
	
	t.Run("List links with admin auth", func(t *testing.T) {
		// Without auth
		resp, err := client.Get(ts.URL + "/api/links")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401 without auth, got %d", resp.StatusCode)
		}
		
		// With auth
		req, _ := http.NewRequest("GET", ts.URL+"/api/links", nil)
		req.Header.Set("Authorization", "Bearer admin-secret")
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 with auth, got %d", resp.StatusCode)
		}
		
		var links []Link
		if err := json.NewDecoder(resp.Body).Decode(&links); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		
		if len(links) < 2 {
			t.Errorf("Expected at least 2 links, got %d", len(links))
		}
		
		// Verify links are sorted by creation time
		for i := 1; i < len(links); i++ {
			if links[i].CreatedAt.Before(links[i-1].CreatedAt) {
				t.Error("Links should be sorted by creation time (oldest first)")
			}
		}
	})
	
	t.Run("Delete link with admin auth", func(t *testing.T) {
		// Create a link to delete
		body, _ := json.Marshal(map[string]string{
			"url":   "https://example.com/delete-me",
			"alias": "delete-test",
		})
		
		resp, err := client.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
		
		// Delete without auth
		req, _ := http.NewRequest("DELETE", ts.URL+"/api/links/delete-test", nil)
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
		
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401 without auth, got %d", resp.StatusCode)
		}
		
		// Delete with auth
		req, _ = http.NewRequest("DELETE", ts.URL+"/api/links/delete-test", nil)
		req.Header.Set("Authorization", "Bearer admin-secret")
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
		
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d", resp.StatusCode)
		}
		
		// Verify link is gone
		resp, err = client.Get(ts.URL + "/delete-test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
		
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for deleted link, got %d", resp.StatusCode)
		}
	})
	
	t.Run("Test persistence across restarts", func(t *testing.T) {
		// Create a link
		body, _ := json.Marshal(map[string]string{
			"url":   "https://example.com/persist",
			"alias": "persist-test",
		})
		
		resp, err := client.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
		
		// Increment visits
		resp, err = client.Get(ts.URL + "/persist-test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
		
		// Create new store with same file (simulating restart)
		store2, err := NewStore("integration.json")
		if err != nil {
			t.Fatalf("Failed to create new store: %v", err)
		}
		defer store2.Close()
		
		// Verify link still exists with correct visit count
		link, exists := store2.Get("persist-test")
		if !exists {
			t.Fatal("Link should persist after restart")
		}
		if link.URL != "https://example.com/persist" {
			t.Errorf("Expected URL https://example.com/persist, got %s", link.URL)
		}
		if link.Visits != 1 {
			t.Errorf("Expected 1 visit after restart, got %d", link.Visits)
		}
	})
}