package main

import (
	"os"
	"testing"
	"time"
)

func TestNewFileStore(t *testing.T) {
	path := "test_data.json"
	defer os.Remove(path)

	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.(*fileStore).close()

	if store == nil {
		t.Fatal("Store should not be nil")
	}
}

func TestStoreCreateAndGet(t *testing.T) {
	path := "test_crud.json"
	defer os.Remove(path)

	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.(*fileStore).close()

	link := Link{
		Code:      "abc123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	err = store.Create(link)
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	got, exists := store.Get("abc123")
	if !exists {
		t.Fatal("Link should exist")
	}
	if got.URL != link.URL {
		t.Fatalf("Got URL %q, want %q", got.URL, link.URL)
	}
}

func TestStoreGetAll(t *testing.T) {
	path := "test_getall.json"
	defer os.Remove(path)

	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	fs := store.(*fileStore)
	defer fs.close()

	// Add multiple links with different timestamps
	link1 := Link{
		Code:      "link1",
		URL:       "https://example.com/1",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		Visits:    0,
	}
	link2 := Link{
		Code:      "link2",
		URL:       "https://example.com/2",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
		Visits:    0,
	}
	link3 := Link{
		Code:      "link3",
		URL:       "https://example.com/3",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Create in non-chronological order
	fs.Create(link3)
	fs.Create(link1)
	fs.Create(link2)

	all := fs.GetAll()
	if len(all) != 3 {
		t.Fatalf("Got %d links, want 3", len(all))
	}

	// Check ordering (oldest first)
	if all[0].Code != "link1" {
		t.Fatalf("First link should be oldest, got %q", all[0].Code)
	}
	if all[1].Code != "link2" {
		t.Fatalf("Second link should be middle, got %q", all[1].Code)
	}
	if all[2].Code != "link3" {
		t.Fatalf("Third link should be newest, got %q", all[2].Code)
	}
}

func TestStoreDelete(t *testing.T) {
	path := "test_delete.json"
	defer os.Remove(path)

	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	fs := store.(*fileStore)
	defer fs.close()

	link := Link{
		Code:      "todelete",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	err = fs.Create(link)
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Verify it exists
	_, exists := fs.Get("todelete")
	if !exists {
		t.Fatal("Link should exist before delete")
	}

	// Delete it
	err = fs.Delete("todelete")
	if err != nil {
		t.Fatalf("Failed to delete link: %v", err)
	}

	// Verify it's gone
	_, exists = fs.Get("todelete")
	if exists {
		t.Fatal("Link should not exist after delete")
	}

	// Try to delete non-existent link
	err = fs.Delete("nonexistent")
	if err == nil {
		t.Fatal("Should error when deleting non-existent link")
	}
}

func TestStoreDuplicateCreate(t *testing.T) {
	path := "test_duplicate.json"
	defer os.Remove(path)

	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	fs := store.(*fileStore)
	defer fs.close()

	link := Link{
		Code:      "duplicate",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	err = fs.Create(link)
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Try to create duplicate
	err = fs.Create(link)
	if err == nil {
		t.Fatal("Should error when creating duplicate link")
	}
}

func TestStoreSaveAndLoad(t *testing.T) {
	path := "test_save_load.json"
	defer os.Remove(path)

	// Create store and add a link
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	fs := store.(*fileStore)
	defer fs.close()

	link := Link{
		Code:      "abc123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	err = fs.Create(link)
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Create new store instance to test loading
	store2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}
	fs2 := store2.(*fileStore)
	defer fs2.close()

	got, exists := fs2.Get("abc123")
	if !exists {
		t.Fatal("Link should exist after reload")
	}
	if got.URL != link.URL {
		t.Fatalf("Got URL %q, want %q", got.URL, link.URL)
	}
}

func TestStoreIncrementVisits(t *testing.T) {
	path := "test_visits.json"
	defer os.Remove(path)

	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	fs := store.(*fileStore)
	defer fs.close()

	link := Link{
		Code:      "test123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	err = fs.Create(link)
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Increment visits
	err = fs.IncrementVisits("test123")
	if err != nil {
		t.Fatalf("Failed to increment visits: %v", err)
	}

	got, exists := fs.Get("test123")
	if !exists {
		t.Fatal("Link should exist")
	}
	if got.Visits != 1 {
		t.Fatalf("Got %d visits, want 1", got.Visits)
	}
}