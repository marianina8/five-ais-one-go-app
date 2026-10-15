package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.json")

	t.Run("NewStore with non-existent file", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		if len(store.GetAllLinks()) != 0 {
			t.Errorf("Expected empty store, got %d links", len(store.GetAllLinks()))
		}
	})

	t.Run("Create and retrieve link", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		link, err := store.CreateLink("https://example.com", "test123")
		if err != nil {
			t.Fatalf("CreateLink failed: %v", err)
		}

		if link.Code != "test123" {
			t.Errorf("Expected code 'test123', got %q", link.Code)
		}

		if link.URL != "https://example.com" {
			t.Errorf("Expected URL 'https://example.com', got %q", link.URL)
		}

		if link.Visits != 0 {
			t.Errorf("Expected 0 visits, got %d", link.Visits)
		}

		retrieved, err := store.GetLink("test123")
		if err != nil {
			t.Fatalf("GetLink failed: %v", err)
		}

		if retrieved.Code != link.Code {
			t.Errorf("Retrieved link code mismatch: expected %q, got %q", link.Code, retrieved.Code)
		}
	})

	t.Run("Create link with duplicate alias", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		_, err = store.CreateLink("https://example.com", "duplicate")
		if err != nil {
			t.Fatalf("First CreateLink failed: %v", err)
		}

		_, err = store.CreateLink("https://example.org", "duplicate")
		if err == nil {
			t.Fatal("Expected error for duplicate alias, got none")
		}
	})

	t.Run("Create link with invalid URL", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		_, err = store.CreateLink("invalid-url", "")
		if err == nil {
			t.Fatal("Expected error for invalid URL, got none")
		}
	})

	t.Run("Increment visits", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		_, err = store.CreateLink("https://example.com", "visittest")
		if err != nil {
			t.Fatalf("CreateLink failed: %v", err)
		}

		for i := 0; i < 5; i++ {
			err = store.IncrementVisits("visittest")
			if err != nil {
				t.Fatalf("IncrementVisits failed on iteration %d: %v", i, err)
			}
		}

		link, err := store.GetLink("visittest")
		if err != nil {
			t.Fatalf("GetLink failed: %v", err)
		}

		if link.Visits != 5 {
			t.Errorf("Expected 5 visits, got %d", link.Visits)
		}
	})

	t.Run("Get non-existent link", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		_, err = store.GetLink("nonexistent")
		if err == nil {
			t.Fatal("Expected error for non-existent link, got none")
		}
	})

	t.Run("Delete link", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		_, err = store.CreateLink("https://example.com", "todelete")
		if err != nil {
			t.Fatalf("CreateLink failed: %v", err)
		}

		err = store.DeleteLink("todelete")
		if err != nil {
			t.Fatalf("DeleteLink failed: %v", err)
		}

		_, err = store.GetLink("todelete")
		if err == nil {
			t.Fatal("Expected error after deleting link, got none")
		}
	})

	t.Run("Delete non-existent link", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		err = store.DeleteLink("nonexistent")
		if err == nil {
			t.Fatal("Expected error deleting non-existent link, got none")
		}
	})

	t.Run("Persistence across restarts", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "persist.json")

		// Create store and add a link
		store1, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}

		_, err = store1.CreateLink("https://example.com", "persistent")
		if err != nil {
			store1.Close()
			t.Fatalf("CreateLink failed: %v", err)
		}
		store1.Close()

		// Create new store from same file
		store2, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed on second instance: %v", err)
		}
		defer store2.Close()

		link, err := store2.GetLink("persistent")
		if err != nil {
			t.Fatalf("GetLink failed after restart: %v", err)
		}

		if link.URL != "https://example.com" {
			t.Errorf("Persisted link URL mismatch: expected 'https://example.com', got %q", link.URL)
		}
	})

	t.Run("GetAllLinks ordering", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		// Clear any existing links
		store.links = make(map[string]*Link)

		// Create links with slight time differences
		store.CreateLink("https://example1.com", "link1")
		time.Sleep(1 * time.Millisecond)
		store.CreateLink("https://example2.com", "link2")
		time.Sleep(1 * time.Millisecond)
		store.CreateLink("https://example3.com", "link3")

		links := store.GetAllLinks()

		if len(links) != 3 {
			t.Fatalf("Expected 3 links, got %d", len(links))
		}

		// Note: GetAllLinks doesn't sort - sorting is done in the server
		// We need to verify all links are present
		seen := make(map[string]bool)
		for _, link := range links {
			seen[link.Code] = true
		}

		if !seen["link1"] || !seen["link2"] || !seen["link3"] {
			t.Errorf("Not all links present in GetAllLinks. Got: %v", seen)
		}
	})

	t.Run("Generate random code when no alias", func(t *testing.T) {
		store, err := NewStore(tmpFile)
		if err != nil {
			t.Fatalf("NewStore failed: %v", err)
		}
		defer store.Close()

		link, err := store.CreateLink("https://example.com", "")
		if err != nil {
			t.Fatalf("CreateLink failed: %v", err)
		}

		if len(link.Code) != 7 {
			t.Errorf("Expected random code length 7, got %d (%q)", len(link.Code), link.Code)
		}
	})
}