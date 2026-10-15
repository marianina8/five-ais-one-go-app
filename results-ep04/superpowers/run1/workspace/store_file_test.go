package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStore_Persistence(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	// Create first store and add a link
	s1, err := newFileStore(dataFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s1.Close()

	link := Link{
		Code:      "test",
		URL:       "https://example.com",
		CreatedAt: time.Now(),
		Visits:    0,
	}

	err = s1.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Close first store
	s1.Close()

	// Create second store from same file
	s2, err := newFileStore(dataFile)
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}
	defer s2.Close()

	// Should retrieve the link
	retrieved, err := s2.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Code != "test" {
		t.Errorf("expected code 'test', got %q", retrieved.Code)
	}
	if retrieved.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %q", retrieved.URL)
	}
}

func TestFileStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	s, err := newFileStore(dataFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	// Test will be expanded later for concurrency testing
	// For now, just verify the store can be created
	_ = s
}

func TestFileStore_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")

	// Create empty file
	f, err := os.Create(dataFile)
	if err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}
	f.Close()

	s, err := newFileStore(dataFile)
	if err != nil {
		t.Fatalf("Failed to create store from empty file: %v", err)
	}
	defer s.Close()

	// Should have no links
	links := s.List()
	if len(links) != 0 {
		t.Errorf("expected 0 links in empty store, got %d", len(links))
	}
}

func TestFileStore_NonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "nonexistent.json")

	s, err := newFileStore(dataFile)
	if err != nil {
		t.Fatalf("Failed to create store with nonexistent file: %v", err)
	}
	defer s.Close()

	// Store should be created successfully
	_ = s
}