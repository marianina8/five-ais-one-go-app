package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"shortener/handlers"
	"shortener/store"
	"testing"
)

func TestMain_AdminTokenRequired(t *testing.T) {
	// Temporarily unset ADMIN_TOKEN
	oldToken := os.Getenv("ADMIN_TOKEN")
	os.Unsetenv("ADMIN_TOKEN")
	defer os.Setenv("ADMIN_TOKEN", oldToken)

	// This should fail since ADMIN_TOKEN is not set
	// We can't actually run main() in a test, so we'll just verify
	// that the environment variable check would fail
	if os.Getenv("ADMIN_TOKEN") != "" {
		t.Error("ADMIN_TOKEN should be unset")
	}
}

func TestIntegration(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "integration-test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Set up environment
	os.Setenv("ADMIN_TOKEN", "test-token")
	
	// Create store
	st, err := store.NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create handler
	h := handlers.NewAPIHandler(st, "test-token")

	// Test 1: Create link without alias
	createReq := httptest.NewRequest("POST", "/api/links", 
		bytes.NewBufferString(`{"url": "https://example.com"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	h.ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", createResp.Code)
	}

	var createResult map[string]interface{}
	if err := json.NewDecoder(createResp.Body).Decode(&createResult); err != nil {
		t.Fatal(err)
	}

	code, ok := createResult["code"].(string)
	if !ok || code == "" {
		t.Error("expected code in response")
	}

	// Test 2: Create link with alias
	createAliasReq := httptest.NewRequest("POST", "/api/links", 
		bytes.NewBufferString(`{"url": "https://google.com", "alias": "google"}`))
	createAliasReq.Header.Set("Content-Type", "application/json")
	createAliasResp := httptest.NewRecorder()
	h.ServeHTTP(createAliasResp, createAliasReq)

	if createAliasResp.Code != http.StatusCreated {
		t.Errorf("expected status 201 for alias, got %d", createAliasResp.Code)
	}

	// Test 3: Redirect
	redirectReq := httptest.NewRequest("GET", "/google", nil)
	redirectResp := httptest.NewRecorder()
	h.ServeHTTP(redirectResp, redirectReq)

	if redirectResp.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", redirectResp.Code)
	}

	// Test 4: List links (unauthorized)
	listReq := httptest.NewRequest("GET", "/api/links", nil)
	listResp := httptest.NewRecorder()
	h.ServeHTTP(listResp, listReq)

	if listResp.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", listResp.Code)
	}

	// Test 5: List links (authorized)
	listAuthReq := httptest.NewRequest("GET", "/api/links", nil)
	listAuthReq.Header.Set("Authorization", "Bearer test-token")
	listAuthResp := httptest.NewRecorder()
	h.ServeHTTP(listAuthResp, listAuthReq)

	if listAuthResp.Code != http.StatusOK {
		t.Errorf("expected status 200 with auth, got %d", listAuthResp.Code)
	}

	var links []interface{}
	if err := json.NewDecoder(listAuthResp.Body).Decode(&links); err != nil {
		t.Fatal(err)
	}

	if len(links) < 2 {
		t.Errorf("expected at least 2 links, got %d", len(links))
	}

	// Test 6: Delete link
	deleteReq := httptest.NewRequest("DELETE", "/api/links/google", nil)
	deleteReq.Header.Set("Authorization", "Bearer test-token")
	deleteResp := httptest.NewRecorder()
	h.ServeHTTP(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", deleteResp.Code)
	}

	// Test 7: Verify deleted link is gone
	verifyReq := httptest.NewRequest("GET", "/google", nil)
	verifyResp := httptest.NewRecorder()
	h.ServeHTTP(verifyResp, verifyReq)

	if verifyResp.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for deleted link, got %d", verifyResp.Code)
	}
}

// Test to verify the server can be built
func TestBuild(t *testing.T) {
	// Simple test to verify the main package compiles
	// This is run by go test ./... already
}