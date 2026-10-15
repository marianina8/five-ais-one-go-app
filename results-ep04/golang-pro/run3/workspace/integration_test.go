package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	// Create a temporary data file
	tmpfile, err := os.CreateTemp("", "integration-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	// Set up environment variables
	os.Setenv("ADMIN_TOKEN", "test-admin-token")

	// Start the server in a goroutine
	os.Args = []string{"shortener", "-addr", ":0", "-data", tmpfile.Name()}
	
	// We can't easily start the server in a test without refactoring
	// Instead, let's test the handlers with the actual HTTP patterns
	// This is more of a functional test
	
	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-admin-token",
	}
	
	// Test 1: Create a link
	reqBody := `{"url": "https://example.com", "alias": "test-alias"}`
	req := httptest.NewRequest("POST", "/api/links", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	
	server.handleCreateLink(w, req)
	
	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}
	
	var link Link
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &link); err != nil {
		t.Fatal(err)
	}
	
	if link.Code != "test-alias" {
		t.Errorf("Expected code 'test-alias', got %s", link.Code)
	}
	
	// Test 2: Redirect to the link
	req = httptest.NewRequest("GET", "/test-alias", nil)
	req.SetPathValue("code", "test-alias")
	w = httptest.NewRecorder()
	
	server.handleRedirect(w, req)
	
	resp = w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("Expected status 302, got %d", resp.StatusCode)
	}
	
	location := resp.Header.Get("Location")
	if location != "https://example.com" {
		t.Errorf("Expected Location header 'https://example.com', got %s", location)
	}
	
	// Test 3: List links (admin)
	req = httptest.NewRequest("GET", "/api/links", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w = httptest.NewRecorder()
	
	server.handleListLinks(w, req)
	
	resp = w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	
	var links []Link
	body, _ = io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &links); err != nil {
		t.Fatal(err)
	}
	
	if len(links) != 1 {
		t.Errorf("Expected 1 link, got %d", len(links))
	}
	
	if links[0].Visits != 1 {
		t.Errorf("Expected 1 visit, got %d", links[0].Visits)
	}
	
	// Test 4: Delete link (admin)
	req = httptest.NewRequest("DELETE", "/api/links/test-alias", nil)
	req.SetPathValue("code", "test-alias")
	req.Header.Set("Authorization", "Bearer test-admin-token")
	w = httptest.NewRecorder()
	
	server.handleDeleteLink(w, req)
	
	resp = w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
	
	// Test 5: Verify link is gone
	req = httptest.NewRequest("GET", "/test-alias", nil)
	req.SetPathValue("code", "test-alias")
	w = httptest.NewRecorder()
	
	server.handleRedirect(w, req)
	
	resp = w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404 after deletion, got %d", resp.StatusCode)
	}
}

func TestConcurrentAccess(t *testing.T) {
	// Test concurrent access to ensure thread safety
	tmpfile, err := os.CreateTemp("", "concurrent-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	server := &Server{
		store:      store,
		adminToken: "test-token",
	}

	// Create initial link
	_, err = store.CreateLink("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Run concurrent requests
	done := make(chan bool)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			// Each goroutine does multiple redirects
			for j := 0; j < 5; j++ {
				req := httptest.NewRequest("GET", "/test", nil)
				req.SetPathValue("code", "test")
				w := httptest.NewRecorder()
				server.handleRedirect(w, req)
				resp := w.Result()
				if resp.StatusCode != http.StatusFound {
					errors <- fmt.Errorf("goroutine %d, attempt %d: expected 302, got %d", id, j, resp.StatusCode)
					return
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Error(err)
	}

	// Verify total visits
	link, err := store.GetLink("test")
	if err != nil {
		t.Fatal(err)
	}

	expectedVisits := 50 // 10 goroutines × 5 visits each
	if link.Visits != expectedVisits {
		t.Errorf("Expected %d visits, got %d", expectedVisits, link.Visits)
	}
}

func TestDataPersistence(t *testing.T) {
	// Test that data persists across store instances
	tmpfile, err := os.CreateTemp("", "persistence-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	// Create first store and add a link
	store1, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	
	_, err = store1.CreateLink("https://example.com", "persistent")
	if err != nil {
		t.Fatal(err)
	}
	
	// Close first store
	store1.Close()
	
	// Create second store and check the link exists
	store2, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	
	link, err := store2.GetLink("persistent")
	if err != nil {
		t.Errorf("Link should persist after restart: %v", err)
	}
	
	if link.URL != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got %s", link.URL)
	}
}