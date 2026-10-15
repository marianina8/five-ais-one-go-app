package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "data.json")
	
	// Set admin token
	os.Setenv("ADMIN_TOKEN", "integration-test-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	store, err := NewStore(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, "integration-test-token")

	t.Run("Complete flow", func(t *testing.T) {
		// Test 1: Create a link
		createReqBody := `{"url": "https://integration.example.com", "alias": "integration-test"}`
		req1, _ := http.NewRequest("POST", "/api/links", bytes.NewBufferString(createReqBody))
		req1.Header.Set("Content-Type", "application/json")
		rr1 := httptest.NewRecorder()
		server.ServeHTTP(rr1, req1)
		
		if rr1.Code != http.StatusCreated {
			t.Errorf("Expected status 201 for create, got %d. Body: %s", rr1.Code, rr1.Body.String())
		}
		
		var createdLink map[string]interface{}
		json.Unmarshal(rr1.Body.Bytes(), &createdLink)
		
		if createdLink["code"] != "integration-test" {
			t.Errorf("Expected code 'integration-test', got %v", createdLink["code"])
		}

		// Test 2: Redirect to the link
		req2, _ := http.NewRequest("GET", "/integration-test", nil)
		rr2 := httptest.NewRecorder()
		server.ServeHTTP(rr2, req2)
		
		if rr2.Code != http.StatusFound {
			t.Errorf("Expected status 302 for redirect, got %d", rr2.Code)
		}
		
		if rr2.Header().Get("Location") != "https://integration.example.com" {
			t.Errorf("Expected Location header 'https://integration.example.com', got %s", rr2.Header().Get("Location"))
		}

		// Test 3: List links (admin)
		req3, _ := http.NewRequest("GET", "/api/links", nil)
		req3.Header.Set("Authorization", "Bearer integration-test-token")
		rr3 := httptest.NewRecorder()
		server.ServeHTTP(rr3, req3)
		
		if rr3.Code != http.StatusOK {
			t.Errorf("Expected status 200 for list, got %d. Body: %s", rr3.Code, rr3.Body.String())
		}
		
		var links []map[string]interface{}
		json.Unmarshal(rr3.Body.Bytes(), &links)
		
		if len(links) != 1 {
			t.Errorf("Expected 1 link in list, got %d", len(links))
		}
		
		// Check visits increased
		if links[0]["visits"].(float64) != 1 {
			t.Errorf("Expected 1 visit after redirect, got %v", links[0]["visits"])
		}

		// Test 4: Delete link (admin)
		req4, _ := http.NewRequest("DELETE", "/api/links/integration-test", nil)
		req4.Header.Set("Authorization", "Bearer integration-test-token")
		rr4 := httptest.NewRecorder()
		server.ServeHTTP(rr4, req4)
		
		if rr4.Code != http.StatusNoContent {
			t.Errorf("Expected status 204 for delete, got %d. Body: %s", rr4.Code, rr4.Body.String())
		}

		// Test 5: Verify link is gone
		req5, _ := http.NewRequest("GET", "/integration-test", nil)
		rr5 := httptest.NewRecorder()
		server.ServeHTTP(rr5, req5)
		
		if rr5.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 after delete, got %d", rr5.Code)
		}
	})

	t.Run("Create link without alias", func(t *testing.T) {
		createReqBody := `{"url": "https://no-alias.example.com"}`
		req, _ := http.NewRequest("POST", "/api/links", bytes.NewBufferString(createReqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		
		if rr.Code != http.StatusCreated {
			t.Errorf("Expected status 201 for create without alias, got %d. Body: %s", rr.Code, rr.Body.String())
		}
		
		var createdLink map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &createdLink)
		
		code := createdLink["code"].(string)
		if len(code) != 7 {
			t.Errorf("Expected random code length 7, got %d (%s)", len(code), code)
		}
	})

	t.Run("Large URL rejection", func(t *testing.T) {
		// Create a URL longer than 2048 characters
		longURL := "https://example.com/" + string(make([]byte, 2100))
		createReqBody := fmt.Sprintf(`{"url": "%s", "alias": "too-long"}`, longURL)
		req, _ := http.NewRequest("POST", "/api/links", bytes.NewBufferString(createReqBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		
		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for too long URL, got %d. Body: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("Invalid auth token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/links", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401 for wrong token, got %d", rr.Code)
		}
	})
}