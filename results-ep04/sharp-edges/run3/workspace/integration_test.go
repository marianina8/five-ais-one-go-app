package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIntegration_CompleteFlow(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "shortener-integration-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// 1. Create a link
	createBody := `{"url": "https://example.com", "alias": "test"}`
	createReq := httptest.NewRequest("POST", "/api/links", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("Create failed: status = %d", createRR.Code)
	}

	var createdLink Link
	if err := json.Unmarshal(createRR.Body.Bytes(), &createdLink); err != nil {
		t.Fatalf("Failed to parse create response: %v", err)
	}

	// 2. Follow the link
	followReq := httptest.NewRequest("GET", "/test", nil)
	followRR := httptest.NewRecorder()
	handler.ServeHTTP(followRR, followReq)

	if followRR.Code != http.StatusFound {
		t.Errorf("Follow status = %d, want %d", followRR.Code, http.StatusFound)
	}

	// 3. Check visits increased (by listing with auth)
	listReq := httptest.NewRequest("GET", "/api/links", nil)
	listReq.Header.Set("Authorization", "Bearer test-token")
	listRR := httptest.NewRecorder()
	handler.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("List failed: status = %d", listRR.Code)
	}

	var links []Link
	if err := json.Unmarshal(listRR.Body.Bytes(), &links); err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("Got %d links, want 1", len(links))
	}

	if links[0].Visits != 1 {
		t.Errorf("Visits = %d, want 1", links[0].Visits)
	}

	// 4. Delete the link
	deleteReq := httptest.NewRequest("DELETE", "/api/links/test", nil)
	deleteReq.Header.Set("Authorization", "Bearer test-token")
	deleteRR := httptest.NewRecorder()
	handler.ServeHTTP(deleteRR, deleteReq)

	if deleteRR.Code != http.StatusNoContent {
		t.Errorf("Delete status = %d, want %d", deleteRR.Code, http.StatusNoContent)
	}

	// 5. Verify link is gone
	followReq2 := httptest.NewRequest("GET", "/test", nil)
	followRR2 := httptest.NewRecorder()
	handler.ServeHTTP(followRR2, followReq2)

	if followRR2.Code != http.StatusNotFound {
		t.Errorf("Status after delete = %d, want %d", followRR2.Code, http.StatusNotFound)
	}
}

func TestIntegration_Sorting(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "shortener-sorting-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	// Create links in reverse order
	links := []struct {
		alias string
		url   string
	}{
		{"link3", "https://example3.com"},
		{"link2", "https://example2.com"},
		{"link1", "https://example1.com"},
	}

	for _, l := range links {
		body := `{"url": "` + l.url + `", "alias": "` + l.alias + `"}`
		req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("Create %s failed: status = %d", l.alias, rr.Code)
		}
	}

	// List and check order (should be oldest first: link3, link2, link1)
	listReq := httptest.NewRequest("GET", "/api/links", nil)
	listReq.Header.Set("Authorization", "Bearer test-token")
	listRR := httptest.NewRecorder()
	handler.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("List failed: status = %d", listRR.Code)
	}

	var result []Link
	if err := json.Unmarshal(listRR.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("Got %d links, want 3", len(result))
	}

	// Check they're in the order they were created (oldest first)
	for i, expected := range []string{"link3", "link2", "link1"} {
		if result[i].Code != expected {
			t.Errorf("Position %d: got %s, want %s", i, result[i].Code, expected)
		}
	}
}