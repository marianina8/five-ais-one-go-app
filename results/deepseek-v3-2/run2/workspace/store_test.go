package main

import (
	"os"
	"testing"
)

func TestJSONStore(t *testing.T) {
	tmpFile := t.TempDir() + "/test.json"

	store, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	// Test creating a link without alias
	link1, err := store.Create("https://example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if link1.Code == "" {
		t.Error("expected generated code")
	}
	if link1.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", link1.URL)
	}
	if link1.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link1.Visits)
	}

	// Test creating a link with alias
	link2, err := store.Create("https://google.com", "google")
	if err != nil {
		t.Fatal(err)
	}
	if link2.Code != "google" {
		t.Errorf("expected code 'google', got %s", link2.Code)
	}

	// Test conflict on duplicate alias
	_, err = store.Create("https://example.org", "google")
	if err == nil || err.Error() != "alias already taken" {
		t.Errorf("expected 'alias already taken' error, got %v", err)
	}

	// Test Get
	link, err := store.Get(link1.Code)
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", link.URL)
	}

	// Test Visit
	err = store.Visit(link1.Code)
	if err != nil {
		t.Fatal(err)
	}
	link, _ = store.Get(link1.Code)
	if link.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", link.Visits)
	}

	// Test List
	links, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}

	// Test Delete
	err = store.Delete(link1.Code)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(link1.Code)
	if err == nil || err.Error() != "link not found" {
		t.Errorf("expected 'link not found' error, got %v", err)
	}

	// Test persistence
	store2, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	link, err = store2.Get(link2.Code)
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != "https://google.com" {
		t.Errorf("persisted URL mismatch, expected https://google.com, got %s", link.URL)
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com", false},
		{"no scheme", "example.com", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no host", "https://", true},
		{"too long", "https://" + string(make([]byte, 2042)) + ".com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
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
		{"valid alias", "my-page_123", false},
		{"too short", "ab", true},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true}, // 33 chars
		{"reserved api", "api", true},
		{"invalid char", "my@page", true},
		{"valid length 3", "abc", false},
		{"valid length 32", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false}, // 32 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAlias(tt.alias)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAlias(%q) error = %v, wantErr %v", tt.alias, err, tt.wantErr)
			}
		})
	}
}

func TestGenerateUniqueCode(t *testing.T) {
	existing := map[string]*Link{
		"abc123": {},
		"def456": {},
	}

	code, err := generateUniqueCode(existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 7 {
		t.Errorf("expected code length 7, got %d", len(code))
	}
	if _, exists := existing[code]; exists {
		t.Errorf("generated duplicate code: %s", code)
	}
}

func TestStoreConcurrency(t *testing.T) {
	tmpFile := t.TempDir() + "/concurrent.json"
	store, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	// Create initial link
	link, err := store.Create("https://example.com", "concurrent")
	if err != nil {
		t.Fatal(err)
	}

	// Run concurrent visits
	const goroutines = 100
	const visitsPerGoroutine = 10
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < visitsPerGoroutine; j++ {
				if err := store.Visit(link.Code); err != nil {
					errCh <- err
					return
				}
			}
			errCh <- nil
		}()
	}

	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}

	finalLink, err := store.Get(link.Code)
	if err != nil {
		t.Fatal(err)
	}

	expectedVisits := goroutines * visitsPerGoroutine
	if finalLink.Visits != expectedVisits {
		t.Errorf("expected %d visits, got %d", expectedVisits, finalLink.Visits)
	}
}

func TestHandleMissingFile(t *testing.T) {
	// Should create new file if doesn't exist
	tmpFile := t.TempDir() + "/missing.json"
	store, err := NewJSONStore(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	// Store should be empty
	links, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Errorf("expected empty store, got %d links", len(links))
	}
}

func TestFileCorruption(t *testing.T) {
	tmpFile := t.TempDir() + "/corrupt.json"
	// Write invalid JSON
	if err := os.WriteFile(tmpFile, []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewJSONStore(tmpFile)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}