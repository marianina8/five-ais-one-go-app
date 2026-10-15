package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreBasicOperations(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.json")

	// Create new empty store
	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Test initial state
	links := store.ListAll()
	if len(links) != 0 {
		t.Errorf("Expected empty store, got %d links", len(links))
	}

	// Test adding a link
	link := &Link{
		Code:      "abc123",
		URL:       "https://example.com",
		CreatedAt: time.Now(),
		Visits:    0,
	}

	err = store.Add(link)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify link was added
	found, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find link, got nil")
	}
	if found.URL != "https://example.com" {
		t.Errorf("Expected URL https://example.com, got %v", found.URL)
	}

	// Test duplicate code
	err = store.Add(link)
	if err == nil {
		t.Error("Expected error when adding duplicate code")
	}

	// Test incrementing visits
	store.IncrementVisits("abc123")
	updated, _ := store.Get("abc123")
	if updated.Visits != 1 {
		t.Errorf("Expected 1 visit, got %d", updated.Visits)
	}

	// Test deleting link
	err = store.Delete("abc123")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify link is gone
	found, err = store.Get("abc123")
	if err == nil {
		t.Error("Expected error when getting deleted link")
	}
	if found != nil {
		t.Error("Expected nil when getting deleted link")
	}

	// Test persistence by creating new store from same file
	store2, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store2.Close()

	links2 := store2.ListAll()
	if len(links2) != 0 {
		t.Errorf("Expected empty store after delete, got %d links", len(links2))
	}
}

func TestStoreFindAvailableCode(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Add some links
	for i := 0; i < 10; i++ {
		code := generateRandomCode()
		link := &Link{
			Code:      code,
			URL:       "https://example.com",
			CreatedAt: time.Now(),
			Visits:    0,
		}
		err := store.Add(link)
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	// Test finding available code
	code := store.findAvailableCode()
	if code == "" {
		t.Fatal("Expected non-empty code")
	}
	if len(code) != 7 {
		t.Errorf("Expected 7 character code, got %d", len(code))
	}

	// This should be a unique code not already in store
	if _, err := store.Get(code); err == nil {
		t.Errorf("Generated code %s already exists in store", code)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.json")

	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Test that store handles concurrent access
	// This is a basic test; more thorough concurrency testing would be done
	// with the -race flag
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			link := &Link{
				Code:      generateRandomCode(),
				URL:       "https://example.com",
				CreatedAt: time.Now(),
				Visits:    0,
			}
			_ = store.Add(link)
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	links := store.ListAll()
	if len(links) != 5 {
		t.Errorf("Expected 5 links, got %d", len(links))
	}
}

func TestStoreFileCorruption(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.json")

	// Write invalid JSON to file
	if err := os.WriteFile(storePath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	// Should fail to load corrupted file
	_, err := NewStore(storePath)
	if err == nil {
		t.Error("Expected error when loading corrupted JSON")
	}
}

func TestStoreEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.json")

	// Create empty file
	if err := os.WriteFile(storePath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write empty file: %v", err)
	}

	store, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed with empty file: %v", err)
	}
	defer store.Close()

	// Should have empty store
	links := store.ListAll()
	if len(links) != 0 {
		t.Errorf("Expected empty store, got %d links", len(links))
	}
}

func TestStoreJSONRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "test.json")

	// Create and populate store
	store1, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Add some links
	links := []*Link{
		{
			Code:      "abc123",
			URL:       "https://example1.com",
			CreatedAt: time.Now().Add(-time.Hour),
			Visits:    5,
		},
		{
			Code:      "def456",
			URL:       "https://example2.com",
			CreatedAt: time.Now(),
			Visits:    10,
		},
	}

	for _, link := range links {
		if err := store1.Add(link); err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}
	store1.Close()

	// Load store again
	store2, err := NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore failed on reload: %v", err)
	}
	defer store2.Close()

	// Verify data persists
	loadedLinks := store2.ListAll()
	if len(loadedLinks) != 2 {
		t.Fatalf("Expected 2 links, got %d", len(loadedLinks))
	}

	// Sort by creation time for consistent comparison
	if loadedLinks[0].CreatedAt.After(loadedLinks[1].CreatedAt) {
		loadedLinks[0], loadedLinks[1] = loadedLinks[1], loadedLinks[0]
	}

	for i, link := range loadedLinks {
		if link.Code != links[i].Code {
			t.Errorf("Link %d: code mismatch, got %s, want %s", i, link.Code, links[i].Code)
		}
		if link.URL != links[i].URL {
			t.Errorf("Link %d: URL mismatch, got %s, want %s", i, link.URL, links[i].URL)
		}
		if link.Visits != links[i].Visits {
			t.Errorf("Link %d: visits mismatch, got %d, want %d", i, link.Visits, links[i].Visits)
		}
	}
}