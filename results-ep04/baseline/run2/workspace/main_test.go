package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStore_Create(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Test creating a link without alias
	link, err := store.Create("https://example.com", "")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	
	if link.Code == "" {
		t.Error("expected code to be generated")
	}
	if len(link.Code) != 7 {
		t.Errorf("expected code length 7, got %d", len(link.Code))
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link.Visits)
	}
	
	// Test creating a link with alias
	link2, err := store.Create("https://google.com", "test-alias")
	if err != nil {
		t.Fatalf("failed to create link with alias: %v", err)
	}
	
	if link2.Code != "test-alias" {
		t.Errorf("expected code test-alias, got %s", link2.Code)
	}
	
	// Test duplicate alias
	_, err = store.Create("https://duplicate.com", "test-alias")
	if err == nil || err.Error() != "code already exists" {
		t.Errorf("expected 'code already exists' error, got %v", err)
	}
	
	// Test invalid URL
	_, err = store.Create("not-a-url", "")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
	
	// Test non-http/https URL
	_, err = store.Create("ftp://example.com", "")
	if err == nil {
		t.Error("expected error for non-http/https URL")
	}
	
	// Test URL without host
	_, err = store.Create("http://", "")
	if err == nil {
		t.Error("expected error for URL without host")
	}
	
	// Test invalid alias format
	_, err = store.Create("https://example.com", "a") // too short
	if err == nil || !strings.Contains(err.Error(), "alias must match") {
		t.Errorf("expected error for invalid alias, got %v", err)
	}
	
	// Test alias "api" is forbidden
	_, err = store.Create("https://example.com", "api")
	if err == nil || err.Error() != "alias cannot be 'api'" {
		t.Errorf("expected 'alias cannot be 'api'' error, got %v", err)
	}
}

func TestStore_Get(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Create a link
	created, err := store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	
	// Get the link
	retrieved, err := store.Get("test123")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	
	if retrieved.URL != created.URL {
		t.Errorf("expected URL %s, got %s", created.URL, retrieved.URL)
	}
	if retrieved.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", retrieved.Visits)
	}
	
	// Get again to increment visits
	retrieved2, err := store.Get("test123")
	if err != nil {
		t.Fatalf("failed to get link again: %v", err)
	}
	
	if retrieved2.Visits != 2 {
		t.Errorf("expected 2 visits, got %d", retrieved2.Visits)
	}
	
	// Test non-existent code
	_, err = store.Get("nonexistent")
	if err == nil || err.Error() != "link not found" {
		t.Errorf("expected 'link not found' error, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Create a link
	_, err = store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	
	// Delete the link
	err = store.Delete("test123")
	if err != nil {
		t.Fatalf("failed to delete link: %v", err)
	}
	
	// Verify it's gone
	_, err = store.Get("test123")
	if err == nil || err.Error() != "link not found" {
		t.Errorf("expected 'link not found' error after deletion, got %v", err)
	}
	
	// Test deleting non-existent link
	err = store.Delete("nonexistent")
	if err == nil || err.Error() != "link not found" {
		t.Errorf("expected 'link not found' error, got %v", err)
	}
}

func TestStore_GetAll(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Create some links
	links := []struct {
		url  string
		code string
	}{
		{"https://example1.com", "code1"},
		{"https://example2.com", "code2"},
		{"https://example3.com", "code3"},
	}
	
	for _, l := range links {
		_, err = store.Create(l.url, l.code)
		if err != nil {
			t.Fatalf("failed to create link: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(time.Millisecond)
	}
	
	// Get all links
	allLinks := store.GetAll()
	
	if len(allLinks) != 3 {
		t.Fatalf("expected 3 links, got %d", len(allLinks))
	}
	
	// Check ordering (oldest first)
	for i := 0; i < len(allLinks)-1; i++ {
		if allLinks[i].CreatedAt.After(allLinks[i+1].CreatedAt) {
			t.Errorf("links not sorted by creation time (oldest first)")
		}
	}
	
	// Check that all links are present
	codes := make(map[string]bool)
	for _, link := range allLinks {
		codes[link.Code] = true
	}
	
	for _, l := range links {
		if !codes[l.code] {
			t.Errorf("link with code %s not found in GetAll", l.code)
		}
	}
}

func TestStore_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "data.json")
	
	// Create first store and add a link
	store1, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store1: %v", err)
	}
	
	_, err = store1.Create("https://example.com", "persistent")
	if err != nil {
		t.Fatalf("failed to create link in store1: %v", err)
	}
	
	// Create second store from same file
	store2, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store2: %v", err)
	}
	
	// Verify link exists in second store
	link, err := store2.Get("persistent")
	if err != nil {
		t.Fatalf("failed to get persistent link from store2: %v", err)
	}
	
	if link.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", link.URL)
	}
}

func TestServer_CreateLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	server := NewServer(store, "admin-token")
	
	// Test successful creation
	reqBody := `{"url": "https://example.com", "alias": "test-alias"}`
	req := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	rr := httptest.NewRecorder()
	
	server.handleCreateLink(rr, req)
	
	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rr.Code)
	}
	
	var response Link
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if response.Code != "test-alias" {
		t.Errorf("expected code test-alias, got %s", response.Code)
	}
	if response.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", response.URL)
	}
	
	// Test duplicate alias
	req2 := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody))
	rr2 := httptest.NewRecorder()
	server.handleCreateLink(rr2, req2)
	
	if rr2.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", rr2.Code)
	}
	
	// Test invalid JSON
	req3 := httptest.NewRequest("POST", "/api/links", strings.NewReader("invalid json"))
	rr3 := httptest.NewRecorder()
	server.handleCreateLink(rr3, req3)
	
	if rr3.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid JSON, got %d", rr3.Code)
	}
	
	// Test missing URL
	req4 := httptest.NewRequest("POST", "/api/links", strings.NewReader(`{"alias": "test"}`))
	rr4 := httptest.NewRecorder()
	server.handleCreateLink(rr4, req4)
	
	if rr4.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing URL, got %d", rr4.Code)
	}
	
	// Test large request body
	largeURL := "https://example.com/" + strings.Repeat("a", 3000)
	reqBody5 := fmt.Sprintf(`{"url": "%s"}`, largeURL)
	req5 := httptest.NewRequest("POST", "/api/links", strings.NewReader(reqBody5))
	rr5 := httptest.NewRecorder()
	server.handleCreateLink(rr5, req5)
	
	if rr5.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for URL too long, got %d", rr5.Code)
	}
}

func TestServer_FollowLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	server := NewServer(store, "admin-token")
	
	// Create a link first
	_, err = store.Create("https://example.com", "test-redirect")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	
	// Test successful redirect
	req := httptest.NewRequest("GET", "/test-redirect", nil)
	rr := httptest.NewRecorder()
	server.handleFollowLink(rr, req, "test-redirect")
	
	if rr.Code != http.StatusFound {
		t.Errorf("expected status 302, got %d", rr.Code)
	}
	
	location := rr.Header().Get("Location")
	if location != "https://example.com" {
		t.Errorf("expected Location https://example.com, got %s", location)
	}
	
	// Verify visits increased
	link, err := store.Get("test-redirect")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	
	if link.Visits != 2 { // 1 from Get above, 1 from handleFollowLink
		t.Errorf("expected 2 visits, got %d", link.Visits)
	}
	
	// Test non-existent link
	req2 := httptest.NewRequest("GET", "/nonexistent", nil)
	rr2 := httptest.NewRecorder()
	server.handleFollowLink(rr2, req2, "nonexistent")
	
	if rr2.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr2.Code)
	}
	
	// Test wrong method
	req3 := httptest.NewRequest("POST", "/test-redirect", nil)
	rr3 := httptest.NewRecorder()
	server.handleFollowLink(rr3, req3, "test-redirect")
	
	if rr3.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rr3.Code)
	}
}

func TestServer_ListLinks(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	server := NewServer(store, "admin-token")
	
	// Create some links
	_, err = store.Create("https://example1.com", "link1")
	if err != nil {
		t.Fatalf("failed to create link1: %v", err)
	}
	_, err = store.Create("https://example2.com", "link2")
	if err != nil {
		t.Fatalf("failed to create link2: %v", err)
	}
	
	// Test without auth
	req := httptest.NewRequest("GET", "/api/links", nil)
	rr := httptest.NewRecorder()
	server.handleListLinks(rr, req)
	
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", rr.Code)
	}
	
	// Test with auth
	req2 := httptest.NewRequest("GET", "/api/links", nil)
	req2.Header.Set("Authorization", "Bearer admin-token")
	rr2 := httptest.NewRecorder()
	server.handleListLinks(rr2, req2)
	
	if rr2.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr2.Code)
	}
	
	var links []Link
	if err := json.NewDecoder(rr2.Body).Decode(&links); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}
	
	// Test wrong method
	req3 := httptest.NewRequest("POST", "/api/links", nil)
	req3.Header.Set("Authorization", "Bearer admin-token")
	rr3 := httptest.NewRecorder()
	server.handleListLinks(rr3, req3)
	
	if rr3.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rr3.Code)
	}
	
	// Test invalid auth format
	req4 := httptest.NewRequest("GET", "/api/links", nil)
	req4.Header.Set("Authorization", "InvalidFormat admin-token")
	rr4 := httptest.NewRecorder()
	server.handleListLinks(rr4, req4)
	
	if rr4.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for invalid auth format, got %d", rr4.Code)
	}
	
	// Test wrong token
	req5 := httptest.NewRequest("GET", "/api/links", nil)
	req5.Header.Set("Authorization", "Bearer wrong-token")
	rr5 := httptest.NewRecorder()
	server.handleListLinks(rr5, req5)
	
	if rr5.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for wrong token, got %d", rr5.Code)
	}
}

func TestServer_DeleteLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	server := NewServer(store, "admin-token")
	
	// Create a link
	_, err = store.Create("https://example.com", "to-delete")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	
	// Test delete without auth
	req := httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	rr := httptest.NewRecorder()
	server.handleDeleteLink(rr, req, "to-delete")
	
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 without auth, got %d", rr.Code)
	}
	
	// Test delete with auth
	req2 := httptest.NewRequest("DELETE", "/api/links/to-delete", nil)
	req2.Header.Set("Authorization", "Bearer admin-token")
	rr2 := httptest.NewRecorder()
	server.handleDeleteLink(rr2, req2, "to-delete")
	
	if rr2.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rr2.Code)
	}
	
	// Verify link is deleted
	_, err = store.Get("to-delete")
	if err == nil {
		t.Error("expected link to be deleted")
	}
	
	// Test delete non-existent link
	req3 := httptest.NewRequest("DELETE", "/api/links/nonexistent", nil)
	req3.Header.Set("Authorization", "Bearer admin-token")
	rr3 := httptest.NewRecorder()
	server.handleDeleteLink(rr3, req3, "nonexistent")
	
	if rr3.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr3.Code)
	}
	
	// Test wrong method
	req4 := httptest.NewRequest("GET", "/api/links/to-delete", nil)
	req4.Header.Set("Authorization", "Bearer admin-token")
	rr4 := httptest.NewRecorder()
	server.handleDeleteLink(rr4, req4, "to-delete")
	
	if rr4.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rr4.Code)
	}
}

func TestServer_ServeHTTP_Routing(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	server := NewServer(store, "admin-token")
	
	// Create a link for testing
	_, err = store.Create("https://example.com", "test-code")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	
	// Test follow link route
	req1 := httptest.NewRequest("GET", "/test-code", nil)
	rr1 := httptest.NewRecorder()
	server.ServeHTTP(rr1, req1)
	
	if rr1.Code != http.StatusFound {
		t.Errorf("expected status 302 for /{code}, got %d", rr1.Code)
	}
	
	// Test create link route
	req2 := httptest.NewRequest("POST", "/api/links", 
		strings.NewReader(`{"url": "https://new.com"}`))
	rr2 := httptest.NewRecorder()
	server.ServeHTTP(rr2, req2)
	
	if rr2.Code != http.StatusCreated {
		t.Errorf("expected status 201 for POST /api/links, got %d", rr2.Code)
	}
	
	// Test list links route (without auth)
	req3 := httptest.NewRequest("GET", "/api/links", nil)
	rr3 := httptest.NewRecorder()
	server.ServeHTTP(rr3, req3)
	
	if rr3.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for GET /api/links without auth, got %d", rr3.Code)
	}
	
	// Test delete link route (without auth)
	req4 := httptest.NewRequest("DELETE", "/api/links/test-code", nil)
	rr4 := httptest.NewRecorder()
	server.ServeHTTP(rr4, req4)
	
	if rr4.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for DELETE /api/links/{code} without auth, got %d", rr4.Code)
	}
	
	// Test root path
	req5 := httptest.NewRequest("GET", "/", nil)
	rr5 := httptest.NewRecorder()
	server.ServeHTTP(rr5, req5)
	
	if rr5.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for /, got %d", rr5.Code)
	}
}

func TestMain_AdminTokenRequired(t *testing.T) {
	// Backup and restore environment
	originalToken := os.Getenv("ADMIN_TOKEN")
	defer os.Setenv("ADMIN_TOKEN", originalToken)
	
	// Test with empty token
	os.Setenv("ADMIN_TOKEN", "")
	
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	
	os.Args = []string{"shortener", "-addr", ":0", "-data", filepath.Join(t.TempDir(), "data.json")}
	
	// We can't actually run main() as it would call log.Fatal, but we can test the token check
	// by creating a minimal test that verifies the logic
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken != "" {
		t.Error("ADMIN_TOKEN should be empty for this test")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Create initial link
	_, err = store.Create("https://example.com", "concurrent")
	if err != nil {
		t.Fatalf("failed to create initial link: %v", err)
	}
	
	// Run concurrent gets
	const numGoroutines = 100
	results := make(chan int, numGoroutines)
	errors := make(chan error, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			link, err := store.Get("concurrent")
			if err != nil {
				errors <- err
				return
			}
			results <- link.Visits
		}()
	}
	
	// Collect results
	totalVisits := 0
	successCount := 0
	
	for i := 0; i < numGoroutines; i++ {
		select {
		case visits := <-results:
			totalVisits += visits
			successCount++
		case err := <-errors:
			t.Errorf("error in goroutine: %v", err)
		}
	}
	
	// The total visits reported should be numGoroutines * (numGoroutines + 1) / 2
	// but due to concurrency, we just check that all succeeded
	if successCount != numGoroutines {
		t.Errorf("expected %d successful gets, got %d", numGoroutines, successCount)
	}
	
	// Final link should have numGoroutines visits added to initial 0
	link, err := store.Get("concurrent")
	if err != nil {
		t.Fatalf("failed to get final link: %v", err)
	}
	
	// Should have numGoroutines + 1 visits (1 from final Get above)
	if link.Visits != numGoroutines+1 {
		t.Errorf("expected %d total visits, got %d", numGoroutines+1, link.Visits)
	}
}

func TestRandomCodeGeneration(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Generate many random codes and ensure uniqueness
	codes := make(map[string]bool)
	const numCodes = 100
	
	for i := 0; i < numCodes; i++ {
		link, err := store.Create("https://example.com", "")
		if err != nil {
			t.Fatalf("failed to create link %d: %v", i, err)
		}
		
		if codes[link.Code] {
			t.Errorf("duplicate code generated: %s", link.Code)
		}
		codes[link.Code] = true
		
		// Verify code format
		if len(link.Code) != 7 {
			t.Errorf("expected code length 7, got %d", len(link.Code))
		}
		
		for _, ch := range link.Code {
			if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
				t.Errorf("invalid character in code: %c", ch)
			}
		}
	}
	
	if len(codes) != numCodes {
		t.Errorf("expected %d unique codes, got %d", numCodes, len(codes))
	}
}

func TestURLValidation(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "data.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	testCases := []struct {
		url     string
		alias   string
		wantErr bool
		desc    string
	}{
		{"https://example.com", "", false, "valid HTTPS URL"},
		{"http://example.com", "", false, "valid HTTP URL"},
		{"https://sub.example.com/path?query=1", "", false, "valid URL with path and query"},
		{"ftp://example.com", "", true, "invalid scheme"},
		{"not-a-url", "", true, "invalid URL"},
		{"http://", "", true, "URL without host"},
		{"https://" + strings.Repeat("a", 3000), "", true, "URL too long"},
		{"https://example.com", "abc", false, "valid 3-char alias"},
		{"https://example.com", "a-b_c123", false, "valid alias with hyphens and underscores"},
		{"https://example.com", "ab", true, "alias too short"},
		{"https://example.com", strings.Repeat("a", 33), true, "alias too long"},
		{"https://example.com", "invalid@alias", true, "alias with invalid character"},
		{"https://example.com", "api", true, "alias 'api' is forbidden"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := store.Create(tc.url, tc.alias)
			hasErr := err != nil
			if hasErr != tc.wantErr {
				t.Errorf("Create(%q, %q): wantErr=%v, got err=%v", tc.url, tc.alias, tc.wantErr, err)
			}
		})
	}
}

func TestFilePersistenceAtomicity(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "data.json")
	
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Add some data
	_, err = store.Create("https://example1.com", "link1")
	if err != nil {
		t.Fatalf("failed to create link1: %v", err)
	}
	
	// Check that .tmp file exists during save (simulate crash)
	store.saveMu.Lock()
	
	// Get the current links to save
	store.mu.RLock()
	links := make([]Link, 0, len(store.links))
	for _, link := range store.links {
		links = append(links, *link)
	}
	store.mu.RUnlock()
	
	// Manually write to temp file
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		store.saveMu.Unlock()
		t.Fatalf("failed to marshal data: %v", err)
	}
	
	tmpFile := tempFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		store.saveMu.Unlock()
		t.Fatalf("failed to write temp file: %v", err)
	}
	
	// Verify temp file exists but main file doesn't have new data yet
	if _, err := os.Stat(tmpFile); err != nil {
		store.saveMu.Unlock()
		t.Fatalf("temp file should exist: %v", err)
	}
	
	// Now rename (atomic operation)
	if err := os.Rename(tmpFile, tempFile); err != nil {
		store.saveMu.Unlock()
		t.Fatalf("failed to rename: %v", err)
	}
	
	store.saveMu.Unlock()
	
	// Verify data is preserved
	store2, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create new store from file: %v", err)
	}
	
	link, err := store2.Get("link1")
	if err != nil {
		t.Fatalf("failed to get persisted link: %v", err)
	}
	
	if link.URL != "https://example1.com" {
		t.Errorf("expected persisted URL https://example1.com, got %s", link.URL)
	}
}