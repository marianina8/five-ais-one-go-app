package store

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestStore_CreateAndGetLink(t *testing.T) {
	// Create temp file for testing
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a link
	link, err := store.CreateLink("test123", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Verify link fields
	if link.Code != "test123" {
		t.Errorf("expected code 'test123', got %s", link.Code)
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link.Visits)
	}
	if link.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Retrieve the link
	retrieved, err := store.GetLink("test123")
	if err != nil {
		t.Fatal(err)
	}

	if retrieved.Code != link.Code || retrieved.URL != link.URL {
		t.Error("retrieved link doesn't match created link")
	}
}

func TestStore_DuplicateCode(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create first link
	_, err = store.CreateLink("test123", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Try to create with same code
	_, err = store.CreateLink("test123", "https://another.com")
	if err == nil {
		t.Error("expected error when creating duplicate code")
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a link
	_, err = store.CreateLink("test123", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Increment visits
	err = store.IncrementVisits("test123")
	if err != nil {
		t.Fatal(err)
	}

	// Verify visits incremented
	link, err := store.GetLink("test123")
	if err != nil {
		t.Fatal(err)
	}

	if link.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", link.Visits)
	}
}

func TestStore_ListLinks(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple links
	_, err = store.CreateLink("first", "https://first.com")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond) // Ensure different timestamps

	_, err = store.CreateLink("second", "https://second.com")
	if err != nil {
		t.Fatal(err)
	}

	// List links
	links := store.ListLinks()

	if len(links) != 2 {
		t.Errorf("expected 2 links, got %d", len(links))
	}

	// Verify ordering (oldest first)
	if links[0].Code != "first" || links[1].Code != "second" {
		t.Error("links not sorted by creation time (oldest first)")
	}
}

func TestStore_DeleteLink(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create a link
	_, err = store.CreateLink("test123", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it exists
	_, err = store.GetLink("test123")
	if err != nil {
		t.Fatal(err)
	}

	// Delete it
	err = store.DeleteLink("test123")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it no longer exists
	_, err = store.GetLink("test123")
	if err == nil {
		t.Error("expected error when getting deleted link")
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create store and add a link
	store1, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store1.CreateLink("test123", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Close store and create new one with same file
	store2, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Verify link persisted
	link, err := store2.GetLink("test123")
	if err != nil {
		t.Fatal(err)
	}

	if link.Code != "test123" || link.URL != "https://example.com" {
		t.Error("link not persisted correctly")
	}
}

func TestStore_FileCorruption(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Write invalid JSON
	if err := os.WriteFile(tmpFile.Name(), []byte("{ invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should fail to load
	_, err = NewStore(tmpFile.Name())
	if err == nil {
		t.Error("expected error when loading corrupted file")
	}
}

func TestStore_EmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create empty file
	if err := os.WriteFile(tmpFile.Name(), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Should handle gracefully
	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Should be able to create links
	_, err = store.CreateLink("test123", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
}

func TestStore_AtomicWrite(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create multiple links to trigger multiple saves
	for i := 0; i < 5; i++ {
		code := string(rune('a' + i))
		_, err := store.CreateLink(code, "https://example.com")
		if err != nil {
			t.Fatal(err)
		}

		// Verify file contains valid JSON after each write
		data, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			t.Fatal(err)
		}

		var links []interface{}
		if err := json.Unmarshal(data, &links); err != nil {
			t.Fatalf("file contains invalid JSON after write %d: %v", i, err)
		}
	}
}