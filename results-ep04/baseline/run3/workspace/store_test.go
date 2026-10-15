package main

import (
	"os"
	"testing"
	"time"
)

func TestStore_CreateLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Test creating link without alias
	link, err := store.CreateLink("https://example.com", "")
	if err != nil {
		t.Fatalf("CreateLink failed: %v", err)
	}

	if link.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", link.URL, "https://example.com")
	}
	if link.Visits != 0 {
		t.Errorf("Visits = %d, want 0", link.Visits)
	}
	if link.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if len(link.Code) != 7 {
		t.Errorf("Code length = %d, want 7", len(link.Code))
	}

	// Test creating link with alias
	link2, err := store.CreateLink("https://example.com/2", "my-alias")
	if err != nil {
		t.Fatalf("CreateLink with alias failed: %v", err)
	}

	if link2.Code != "my-alias" {
		t.Errorf("Code = %q, want %q", link2.Code, "my-alias")
	}

	// Test duplicate alias
	_, err = store.CreateLink("https://example.com/3", "my-alias")
	if err != ErrAliasTaken {
		t.Errorf("Duplicate alias error = %v, want %v", err, ErrAliasTaken)
	}

	// Test invalid URL
	_, err = store.CreateLink("ftp://example.com", "")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestStore_GetAndIncrement(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a link
	link, err := store.CreateLink("https://example.com", "test-code")
	if err != nil {
		t.Fatal(err)
	}

	// Get and increment
	retrieved, err := store.GetAndIncrement("test-code")
	if err != nil {
		t.Fatalf("GetAndIncrement failed: %v", err)
	}

	if retrieved.URL != link.URL {
		t.Errorf("URL = %q, want %q", retrieved.URL, link.URL)
	}
	if retrieved.Visits != 1 {
		t.Errorf("Visits = %d, want 1", retrieved.Visits)
	}

	// Increment again
	retrieved2, err := store.GetAndIncrement("test-code")
	if err != nil {
		t.Fatalf("Second GetAndIncrement failed: %v", err)
	}
	if retrieved2.Visits != 2 {
		t.Errorf("Second Visits = %d, want 2", retrieved2.Visits)
	}

	// Test non-existent code
	_, err = store.GetAndIncrement("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent code")
	}
}

func TestStore_ListLinks(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create links in reverse order
	time.Sleep(time.Millisecond)
	store.CreateLink("https://example.com/2", "second")
	time.Sleep(time.Millisecond)
	store.CreateLink("https://example.com/1", "first")

	links := store.ListLinks()

	if len(links) != 2 {
		t.Fatalf("ListLinks returned %d links, want 2", len(links))
	}

	// Should be sorted oldest first - "second" was created first, then "first"
	if links[0].Code != "second" {
		t.Errorf("First (oldest) link code = %q, want %q", links[0].Code, "second")
	}
	if links[1].Code != "first" {
		t.Errorf("Second (newest) link code = %q, want %q", links[1].Code, "first")
	}
}

func TestStore_DeleteLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	store, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a link
	_, err = store.CreateLink("https://example.com", "test-code")
	if err != nil {
		t.Fatal(err)
	}

	// Delete it
	err = store.DeleteLink("test-code")
	if err != nil {
		t.Fatalf("DeleteLink failed: %v", err)
	}

	// Should not exist anymore
	_, err = store.GetAndIncrement("test-code")
	if err == nil {
		t.Error("Link should not exist after deletion")
	}

	// Delete non-existent link
	err = store.DeleteLink("non-existent")
	if err == nil {
		t.Error("Expected error when deleting non-existent link")
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Create store and add a link
	store1, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store1.CreateLink("https://example.com", "persistent")
	if err != nil {
		t.Fatal(err)
	}
	store1.Close()

	// Create new store from same file
	store2, err := NewStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	// Link should still exist
	link, err := store2.GetAndIncrement("persistent")
	if err != nil {
		t.Fatalf("Failed to get persisted link: %v", err)
	}

	if link.URL != "https://example.com" {
		t.Errorf("Persisted URL = %q, want %q", link.URL, "https://example.com")
	}
	if link.Visits != 1 {
		t.Errorf("Persisted visits after increment = %d, want 1", link.Visits)
	}
}