package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_CreateAndGetLink(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create a link
	link, err := store.CreateLink("test123", "https://example.com", "test-alias")
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	if link.Code != "test123" {
		t.Errorf("Expected code 'test123', got '%s'", link.Code)
	}
	if link.URL != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%s'", link.URL)
	}
	if link.Alias != "test-alias" {
		t.Errorf("Expected alias 'test-alias', got '%s'", link.Alias)
	}
	if link.Visits != 0 {
		t.Errorf("Expected 0 visits, got %d", link.Visits)
	}

	// Get the link
	retrieved, err := store.GetLink("test123")
	if err != nil {
		t.Fatalf("GetLink failed: %v", err)
	}

	if retrieved.Code != link.Code {
		t.Errorf("Retrieved code doesn't match")
	}
	if retrieved.URL != link.URL {
		t.Errorf("Retrieved URL doesn't match")
	}

	// Test duplicate code
	_, err = store.CreateLink("test123", "https://example2.com", "")
	if err == nil {
		t.Error("Expected error for duplicate code")
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create a link
	_, err = store.CreateLink("test123", "https://example.com", "")
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	// Increment visits
	for i := 1; i <= 5; i++ {
		err = store.IncrementVisits("test123")
		if err != nil {
			t.Fatalf("IncrementVisits failed on iteration %d: %v", i, err)
		}

		link, err := store.GetLink("test123")
		if err != nil {
			t.Fatalf("GetLink failed: %v", err)
		}

		if link.Visits != i {
			t.Errorf("Expected %d visits, got %d", i, link.Visits)
		}
	}

	// Test increment on non-existent link
	err = store.IncrementVisits("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent link")
	}
}

func TestStore_DeleteLink(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create a link
	_, err = store.CreateLink("test123", "https://example.com", "")
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	// Verify it exists
	_, err = store.GetLink("test123")
	if err != nil {
		t.Fatalf("GetLink failed: %v", err)
	}

	// Delete the link
	err = store.DeleteLink("test123")
	if err != nil {
		t.Fatalf("DeleteLink failed: %v", err)
	}

	// Verify it's gone
	_, err = store.GetLink("test123")
	if err == nil {
		t.Error("Expected error for deleted link")
	}

	// Test delete on non-existent link
	err = store.DeleteLink("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent link")
	}
}

func TestStore_GetAllLinks(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create multiple links with different creation times
	links := []struct {
		code    string
		url     string
		waitMs  int
	}{
		{"first", "https://first.com", 100},
		{"second", "https://second.com", 50},
		{"third", "https://third.com", 0},
	}

	for _, l := range links {
		if l.waitMs > 0 {
			time.Sleep(time.Duration(l.waitMs) * time.Millisecond)
		}
		_, err := store.CreateLink(l.code, l.url, "")
		if err != nil {
			t.Fatalf("CreateLink failed for %s: %v", l.code, err)
		}
	}

	// Get all links
	allLinks := store.GetAllLinks()

	if len(allLinks) != 3 {
		t.Fatalf("Expected 3 links, got %d", len(allLinks))
	}

	// Verify they're sorted oldest first (third, second, first based on wait times)
	if allLinks[0].Code != "third" {
		t.Errorf("Expected oldest link to be 'third', got '%s'", allLinks[0].Code)
	}
	if allLinks[1].Code != "second" {
		t.Errorf("Expected second oldest link to be 'second', got '%s'", allLinks[1].Code)
	}
	if allLinks[2].Code != "first" {
		t.Errorf("Expected newest link to be 'first', got '%s'", allLinks[2].Code)
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	// Create store and add data
	store1, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, err = store1.CreateLink("persist1", "https://persist1.com", "")
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	_, err = store1.CreateLink("persist2", "https://persist2.com", "alias2")
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	// Increment visits
	err = store1.IncrementVisits("persist1")
	if err != nil {
		t.Fatalf("IncrementVisits failed: %v", err)
	}

	err = store1.IncrementVisits("persist1")
	if err != nil {
		t.Fatalf("IncrementVisits failed: %v", err)
	}

	store1.Close()

	// Create new store from same file
	store2, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed on reload: %v", err)
	}
	defer store2.Close()

	// Verify persisted data
	link1, err := store2.GetLink("persist1")
	if err != nil {
		t.Fatalf("GetLink failed for persist1: %v", err)
	}

	if link1.URL != "https://persist1.com" {
		t.Errorf("Expected URL 'https://persist1.com', got '%s'", link1.URL)
	}
	if link1.Visits != 2 {
		t.Errorf("Expected 2 visits for persist1, got %d", link1.Visits)
	}

	link2, err := store2.GetLink("persist2")
	if err != nil {
		t.Fatalf("GetLink failed for persist2: %v", err)
	}

	if link2.URL != "https://persist2.com" {
		t.Errorf("Expected URL 'https://persist2.com', got '%s'", link2.URL)
	}
	if link2.Alias != "alias2" {
		t.Errorf("Expected alias 'alias2', got '%s'", link2.Alias)
	}
	if link2.Visits != 0 {
		t.Errorf("Expected 0 visits for persist2, got %d", link2.Visits)
	}

	// Verify CodeExists works
	if !store2.CodeExists("persist1") {
		t.Error("CodeExists should return true for persist1")
	}
	if store2.CodeExists("nonexistent") {
		t.Error("CodeExists should return false for nonexistent")
	}
}

func TestStore_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	// Create empty file
	file, err := os.Create(dataFile)
	if err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}
	file.Close()

	// Should load without error
	store, err := NewStore(dataFile)
	if err != nil {
		t.Fatalf("NewStore failed with empty file: %v", err)
	}
	defer store.Close()

	// Should be empty
	links := store.GetAllLinks()
	if len(links) != 0 {
		t.Errorf("Expected empty store, got %d links", len(links))
	}
}