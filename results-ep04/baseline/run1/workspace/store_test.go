package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStoreCreateLink(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Test creating a link without alias
	link1, err := store.CreateLink("https://example.com", "")
	if err != nil {
		t.Fatalf("failed to create link without alias: %v", err)
	}
	if link1.Code == "" {
		t.Error("expected non-empty code")
	}
	if len(link1.Code) != 7 {
		t.Errorf("expected code length 7, got %d", len(link1.Code))
	}
	if link1.URL != "https://example.com" {
		t.Errorf("expected URL %q, got %q", "https://example.com", link1.URL)
	}
	if link1.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link1.Visits)
	}

	// Test creating a link with alias
	link2, err := store.CreateLink("https://google.com", "google")
	if err != nil {
		t.Fatalf("failed to create link with alias: %v", err)
	}
	if link2.Code != "google" {
		t.Errorf("expected code %q, got %q", "google", link2.Code)
	}
	if link2.Alias != "google" {
		t.Errorf("expected alias %q, got %q", "google", link2.Alias)
	}

	// Test duplicate alias
	_, err = store.CreateLink("https://example2.com", "google")
	if err != ErrDuplicateCode {
		t.Errorf("expected ErrDuplicateCode for duplicate alias, got %v", err)
	}

	// Test invalid URL
	_, err = store.CreateLink("not-a-url", "")
	if err == nil {
		t.Error("expected error for invalid URL")
	}

	// Test invalid alias (reserved)
	_, err = store.CreateLink("https://example.com", "api")
	if err == nil {
		t.Error("expected error for reserved alias 'api'")
	}

	// Test invalid alias (too short)
	_, err = store.CreateLink("https://example.com", "ab")
	if err == nil {
		t.Error("expected error for alias too short")
	}

	// Test invalid alias (invalid characters)
	_, err = store.CreateLink("https://example.com", "test@test")
	if err == nil {
		t.Error("expected error for alias with invalid characters")
	}
}

func TestStoreGetLink(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create a link
	link1, err := store.CreateLink("https://example.com", "example")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Get the link
	link2, err := store.GetLink("example")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if !reflect.DeepEqual(link1, link2) {
		t.Error("retrieved link doesn't match created link")
	}

	// Test non-existent link
	_, err = store.GetLink("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent link, got %v", err)
	}
}

func TestStoreIncrementVisits(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create a link
	link, err := store.CreateLink("https://example.com", "example")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Increment visits
	url, err := store.IncrementVisits("example")
	if err != nil {
		t.Fatalf("failed to increment visits: %v", err)
	}
	if url != "https://example.com" {
		t.Errorf("expected URL %q, got %q", "https://example.com", url)
	}

	// Check visits were incremented
	link, err = store.GetLink("example")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if link.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", link.Visits)
	}

	// Test non-existent link
	_, err = store.IncrementVisits("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent link, got %v", err)
	}
}

func TestStoreDeleteLink(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create a link
	_, err = store.CreateLink("https://example.com", "example")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Delete the link
	err = store.DeleteLink("example")
	if err != nil {
		t.Fatalf("failed to delete link: %v", err)
	}

	// Verify link is gone
	_, err = store.GetLink("example")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Test delete non-existent link
	err = store.DeleteLink("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for non-existent link, got %v", err)
	}
}

func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	// Create store and add some links
	store1, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	link1, err := store1.CreateLink("https://example.com", "example")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	// Increment visits
	_, err = store1.IncrementVisits("example")
	if err != nil {
		t.Fatalf("failed to increment visits: %v", err)
	}

	store1.Close()

	// Create new store from same file
	store2, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store from file: %v", err)
	}
	defer store2.Close()

	// Verify data was persisted
	link2, err := store2.GetLink("example")
	if err != nil {
		t.Fatalf("failed to get persisted link: %v", err)
	}

	// Check fields
	if link2.Code != link1.Code {
		t.Errorf("code mismatch: got %q, want %q", link2.Code, link1.Code)
	}
	if link2.URL != link1.URL {
		t.Errorf("URL mismatch: got %q, want %q", link2.URL, link1.URL)
	}
	if link2.Visits != 1 {
		t.Errorf("visits mismatch: got %d, want %d", link2.Visits, 1)
	}
}

func TestStoreListLinks(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create links in reverse order to test sorting
	store.CreateLink("https://example1.com", "example1")
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	store.CreateLink("https://example2.com", "example2")
	time.Sleep(1 * time.Millisecond)
	store.CreateLink("https://example3.com", "example3")

	links := store.ListLinks()
	if len(links) != 3 {
		t.Errorf("expected 3 links, got %d", len(links))
	}

	// Check ordering (oldest first)
	for i := 1; i < len(links); i++ {
		if links[i].CreatedAt.Before(links[i-1].CreatedAt) {
			t.Error("links not sorted by creation time (oldest first)")
		}
	}
}

func TestStoreFileAtomicity(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "test.json")

	// Create initial store with some data
	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	_, err = store.CreateLink("https://example.com", "example")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}
	store.Close()

	// Read the file directly to ensure it's valid JSON
	data, err := os.ReadFile(dataFile)
	if err != nil {
		t.Fatalf("failed to read data file: %v", err)
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		t.Fatalf("data file contains invalid JSON: %v", err)
	}

	if len(links) != 1 {
		t.Errorf("expected 1 link in file, got %d", len(links))
	}
}