package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration_CompleteFlow(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")
	
	os.Setenv("ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-admin-token")

	// Test 1: Create a link with alias
	t.Run("create link with alias", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/links",
			strings.NewReader(`{"url": "https://example.com", "alias": "my-page"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp["code"] != "my-page" {
			t.Errorf("expected code 'my-page', got %v", resp["code"])
		}
		if resp["url"] != "https://example.com" {
			t.Errorf("expected url 'https://example.com', got %v", resp["url"])
		}
		if resp["visits"].(float64) != 0 {
			t.Errorf("expected 0 visits, got %v", resp["visits"])
		}
	})

	// Test 2: Duplicate alias should fail
	t.Run("duplicate alias conflict", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/links",
			strings.NewReader(`{"url": "https://example.net", "alias": "my-page"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusConflict {
			t.Errorf("expected 409 for duplicate alias, got %d", rr.Code)
		}
	})

	// Test 3: Create a link without alias (should generate random code)
	t.Run("create link without alias", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/links",
			strings.NewReader(`{"url": "https://example.org"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", rr.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		code := resp["code"].(string)
		if len(code) != 7 {
			t.Errorf("expected random code length 7, got %d", len(code))
		}
	})

	// Test 4: Redirect to created link
	t.Run("redirect to link", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/my-page", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Errorf("expected 302, got %d", rr.Code)
		}
		if location := rr.Header().Get("Location"); location != "https://example.com" {
			t.Errorf("expected Location 'https://example.com', got %s", location)
		}
	})

	// Test 5: List links (admin)
	t.Run("list links admin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/links", nil)
		req.Header.Set("Authorization", "Bearer test-admin-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}

		var links []map[string]interface{}
		if err := json.NewDecoder(rr.Body).Decode(&links); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(links) != 2 {
			t.Errorf("expected 2 links, got %d", len(links))
		}

		// Check visits count increased
		for _, link := range links {
			if link["code"] == "my-page" {
				if link["visits"].(float64) != 1 {
					t.Errorf("expected 1 visit for my-page, got %v", link["visits"])
				}
			}
		}
	})

	// Test 6: Delete link (admin)
	t.Run("delete link admin", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/links/my-page", nil)
		req.Header.Set("Authorization", "Bearer test-admin-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rr.Code)
		}

		// Verify link is deleted
		redirectReq := httptest.NewRequest("GET", "/my-page", nil)
		redirectRR := httptest.NewRecorder()
		handler.ServeHTTP(redirectRR, redirectReq)

		if redirectRR.Code != http.StatusNotFound {
			t.Errorf("deleted link should return 404, got %d", redirectRR.Code)
		}
	})

	// Test 7: Create link with previously deleted alias (should work)
	t.Run("recreate deleted alias", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/links",
			strings.NewReader(`{"url": "https://new-example.com", "alias": "my-page"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected 201 after deleting and recreating, got %d", rr.Code)
		}
	})
}

func TestIntegration_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")
	
	os.Setenv("ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	// Create first store and add a link
	store1, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store1: %v", err)
	}
	handler1 := NewHandler(store1, "test-admin-token")

	req := httptest.NewRequest("POST", "/api/links",
		strings.NewReader(`{"url": "https://persisted.com", "alias": "persist-test"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler1.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("failed to create link: %d", rr.Code)
	}

	store1.Close()

	// Create second store and verify persistence
	store2, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store2: %v", err)
	}
	defer store2.Close()
	
	handler2 := NewHandler(store2, "test-admin-token")

	// Verify link exists
	redirectReq := httptest.NewRequest("GET", "/persist-test", nil)
	redirectRR := httptest.NewRecorder()
	handler2.ServeHTTP(redirectRR, redirectReq)

	if redirectRR.Code != http.StatusFound {
		t.Errorf("persisted link should redirect, got %d", redirectRR.Code)
	}
	if location := redirectRR.Header().Get("Location"); location != "https://persisted.com" {
		t.Errorf("expected Location 'https://persisted.com', got %s", location)
	}
}