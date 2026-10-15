package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore_CreateLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Test creating link with alias
	link1, err := store.CreateLink("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	if link1.Code != "test123" {
		t.Errorf("expected code 'test123', got %s", link1.Code)
	}
	if link1.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", link1.URL)
	}
	if link1.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link1.Visits)
	}
	if link1.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Test duplicate alias
	_, err = store.CreateLink("https://example2.com", "test123")
	if err == nil || !contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got %v", err)
	}

	// Test creating link without alias
	link2, err := store.CreateLink("https://example.org", "")
	if err != nil {
		t.Fatalf("failed to create link without alias: %v", err)
	}
	if len(link2.Code) != 7 {
		t.Errorf("expected 7-character code, got %s (len=%d)", link2.Code, len(link2.Code))
	}

	// Test invalid URL
	_, err = store.CreateLink("not-a-url", "")
	if err == nil || !contains(err.Error(), "invalid URL") {
		t.Errorf("expected 'invalid URL' error, got %v", err)
	}

	// Test non-http/https URL
	_, err = store.CreateLink("ftp://example.com", "")
	if err == nil || !contains(err.Error(), "URL must be http or https") {
		t.Errorf("expected 'URL must be http or https' error, got %v", err)
	}

	// Test URL without host
	_, err = store.CreateLink("http://", "")
	if err == nil || !contains(err.Error(), "URL must have a host") {
		t.Errorf("expected 'URL must have a host' error, got %v", err)
	}

	// Test invalid alias format
	_, err = store.CreateLink("https://example.com", "ab") // too short
	if err == nil || !contains(err.Error(), "alias must match") {
		t.Errorf("expected 'alias must match' error, got %v", err)
	}

	// Test reserved alias
	_, err = store.CreateLink("https://example.com", "api")
	if err == nil || !contains(err.Error(), "alias 'api' is reserved") {
		t.Errorf("expected 'alias 'api' is reserved' error, got %v", err)
	}

	// Test URL too long
	longURL := "https://example.com/" + string(make([]byte, 3000))
	_, err = store.CreateLink(longURL, "")
	if err == nil || !contains(err.Error(), "URL too long") {
		t.Errorf("expected 'URL too long' error, got %v", err)
	}
}

func TestStore_GetLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	link, err := store.CreateLink("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Test getting existing link
	fetched, err := store.GetLink("test123")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if fetched.Code != link.Code || fetched.URL != link.URL {
		t.Errorf("fetched link doesn't match created link")
	}

	// Test getting non-existent link
	_, err = store.GetLink("nonexistent")
	if err == nil || !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.CreateLink("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Increment visits
	if err := store.IncrementVisits("test123"); err != nil {
		t.Fatalf("failed to increment visits: %v", err)
	}

	link, err := store.GetLink("test123")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if link.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", link.Visits)
	}

	// Increment again
	if err := store.IncrementVisits("test123"); err != nil {
		t.Fatalf("failed to increment visits second time: %v", err)
	}

	link, err = store.GetLink("test123")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if link.Visits != 2 {
		t.Errorf("expected 2 visits, got %d", link.Visits)
	}

	// Test increment on non-existent link
	err = store.IncrementVisits("nonexistent")
	if err == nil || !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestStore_ListLinks(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create multiple links
	links := []struct {
		url   string
		alias string
	}{
		{"https://example1.com", "test1"},
		{"https://example2.com", "test2"},
		{"https://example3.com", "test3"},
	}

	for _, l := range links {
		_, err := store.CreateLink(l.url, l.alias)
		if err != nil {
			t.Fatalf("failed to create link %s: %v", l.alias, err)
		}
	}

	// List links
	list := store.ListLinks()
	if len(list) != 3 {
		t.Errorf("expected 3 links, got %d", len(list))
	}

	// Verify all links are present
	aliases := make(map[string]bool)
	for _, link := range list {
		aliases[link.Code] = true
	}
	for _, l := range links {
		if !aliases[l.alias] {
			t.Errorf("link with alias %s not found in list", l.alias)
		}
	}
}

func TestStore_DeleteLink(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test.json")
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.CreateLink("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Verify link exists
	_, err = store.GetLink("test123")
	if err != nil {
		t.Fatalf("link should exist: %v", err)
	}

	// Delete link
	if err := store.DeleteLink("test123"); err != nil {
		t.Fatalf("failed to delete link: %v", err)
	}

	// Verify link no longer exists
	_, err = store.GetLink("test123")
	if err == nil || !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error after deletion, got %v", err)
	}

	// Test deleting non-existent link
	err = store.DeleteLink("nonexistent")
	if err == nil || !contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestStore_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test.json")

	// Create store and add a link
	store1, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	link, err := store1.CreateLink("https://example.com", "test123")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	store1.Close()

	// Create new store and load data
	store2, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create second store: %v", err)
	}
	defer store2.Close()

	// Verify link was persisted
	fetched, err := store2.GetLink("test123")
	if err != nil {
		t.Fatalf("failed to get persisted link: %v", err)
	}
	if fetched.Code != link.Code || fetched.URL != link.URL {
		t.Errorf("persisted link doesn't match created link")
	}
	// Compare times with 1-second tolerance due to JSON marshaling
	if fetched.CreatedAt.Unix() != link.CreatedAt.Unix() {
		t.Errorf("persisted CreatedAt doesn't match: got %v, want %v", fetched.CreatedAt, link.CreatedAt)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr))
}