package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com", false},
		{"invalid scheme", "ftp://example.com", true},
		{"missing scheme", "example.com", true},
		{"missing host", "http://", true},
		{"too long", "http://" + string(make([]byte, 2042)) + ".com", true},
		{"valid with path", "https://example.com/path/to/resource", false},
		{"valid with query", "http://example.com?query=param", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAlias(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		wantErr bool
	}{
		{"valid alias", "my-page-123", false},
		{"too short", "ab", true},
		{"too long", "this-alias-is-way-too-long-for-the-spec", true},
		{"invalid chars", "my@page", true},
		{"reserved api", "api", true},
		{"valid underscore", "my_page", false},
		{"valid numbers", "page123", false},
		{"valid mixed", "My-Page_123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAlias(tt.alias)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAlias() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestJSONStore_Create(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	// Test creating a link with generated code
	link1, err := store.Create("https://example.com", "")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if link1.Code == "" {
		t.Error("Create() returned empty code")
	}
	if link1.URL != "https://example.com" {
		t.Errorf("Create() URL = %v, want https://example.com", link1.URL)
	}
	if link1.Visits != 0 {
		t.Errorf("Create() Visits = %v, want 0", link1.Visits)
	}

	// Test creating a link with alias
	link2, err := store.Create("https://google.com", "my-google")
	if err != nil {
		t.Fatalf("Create() with alias error = %v", err)
	}
	if link2.Code != "my-google" {
		t.Errorf("Create() with alias Code = %v, want my-google", link2.Code)
	}

	// Test duplicate alias
	_, err = store.Create("https://example.org", "my-google")
	if err == nil || !strings.Contains(err.Error(), "alias taken") {
		t.Errorf("Create() duplicate alias error = %v, want error containing 'alias taken'", err)
	}

	// Test invalid URL
	_, err = store.Create("invalid-url", "")
	if err == nil || !strings.Contains(err.Error(), "invalid url") {
		t.Errorf("Create() invalid URL error = %v, want error containing 'invalid url'", err)
	}

	// Test invalid alias
	_, err = store.Create("https://example.com", "@bad!")
	if err == nil || !strings.Contains(err.Error(), "invalid alias") {
		t.Errorf("Create() invalid alias error = %v, want error containing 'invalid alias'", err)
	}
}

func TestJSONStore_Get(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	link, err := store.Create("https://example.com", "test-code")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Test getting existing link
	got, err := store.Get("test-code")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Code != link.Code {
		t.Errorf("Get() Code = %v, want %v", got.Code, link.Code)
	}
	if got.URL != link.URL {
		t.Errorf("Get() URL = %v, want %v", got.URL, link.URL)
	}

	// Test getting non-existent link
	_, err = store.Get("non-existent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Get() non-existent error = %v, want error containing 'not found'", err)
	}
}

func TestJSONStore_IncrementVisits(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	_, err = store.Create("https://example.com", "test-visits")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Increment visits
	for i := 1; i <= 3; i++ {
		if err := store.IncrementVisits("test-visits"); err != nil {
			t.Fatalf("IncrementVisits() %d error = %v", i, err)
		}
	}

	// Check count
	link, err := store.Get("test-visits")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if link.Visits != 3 {
		t.Errorf("Visits = %v, want 3", link.Visits)
	}

	// Test increment non-existent
	err = store.IncrementVisits("non-existent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("IncrementVisits() non-existent error = %v, want error containing 'not found'", err)
	}
}

func TestJSONStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	// Create links
	links := []struct {
		url   string
		alias string
	}{
		{"https://first.com", "first"},
		{"https://second.com", "second"},
		{"https://third.com", "third"},
	}

	// Add small delays to ensure different timestamps
	for _, l := range links {
		_, err := store.Create(l.url, l.alias)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Get list
	got, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Should be sorted by CreatedAt (oldest first)
	if len(got) != 3 {
		t.Fatalf("List() returned %v links, want 3", len(got))
	}

	// Check that it's sorted (oldest first)
	for i := 1; i < len(got); i++ {
		if got[i].CreatedAt.Before(got[i-1].CreatedAt) {
			t.Errorf("List() not sorted: element %d (%v) is before element %d (%v)", 
				i, got[i].CreatedAt, i-1, got[i-1].CreatedAt)
		}
	}
}

func TestJSONStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	_, err = store.Create("https://example.com", "to-delete")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Delete existing
	err = store.Delete("to-delete")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = store.Get("to-delete")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Get() after Delete error = %v, want error containing 'not found'", err)
	}

	// Delete non-existent
	err = store.Delete("non-existent")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Delete() non-existent error = %v, want error containing 'not found'", err)
	}
}

func TestJSONStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	// Create store and add data
	store1, err := NewJSONStore(dataFile)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	_, err = store1.Create("https://example.com", "persistent")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Create new store instance (simulating restart)
	store2, err := NewJSONStore(dataFile)
	if err != nil {
		t.Fatalf("NewJSONStore() second instance error = %v", err)
	}

	// Should find the persisted link
	link, err := store2.Get("persistent")
	if err != nil {
		t.Fatalf("Get() after restart error = %v", err)
	}
	if link.URL != "https://example.com" {
		t.Errorf("Persisted link URL = %v, want https://example.com", link.URL)
	}
}

func TestJSONStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewJSONStore(filepath.Join(tmpDir, "data.json"))
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	// Create initial link
	_, err = store.Create("https://example.com", "concurrent")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Concurrent increments
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				store.IncrementVisits("concurrent")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check final count
	link, err := store.Get("concurrent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if link.Visits != 1000 { // 10 * 100
		t.Errorf("Concurrent visits count = %v, want 1000", link.Visits)
	}
}

func TestGenerateRandomCode(t *testing.T) {
	code1 := generateRandomCode()
	code2 := generateRandomCode()

	if len(code1) != 7 {
		t.Errorf("generateRandomCode() length = %v, want 7", len(code1))
	}
	if len(code2) != 7 {
		t.Errorf("generateRandomCode() length = %v, want 7", len(code2))
	}
	// Very small chance they could be equal, but statistically negligible
	if code1 == code2 {
		t.Error("generateRandomCode() returned same code twice")
	}
}