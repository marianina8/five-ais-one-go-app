package main

import (
	"os"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	// Clean up any test files
	defer os.Remove("test1.json")
	defer os.Remove("test1.json.tmp")

	store, err := NewStore("test1.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("Store should not be nil")
	}
}

func TestStore_CreateAndGet(t *testing.T) {
	defer os.Remove("test2.json")
	defer os.Remove("test2.json.tmp")

	store, err := NewStore("test2.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	link := Link{
		Code:      "test123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Create link
	err = store.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get link
	retrievedLink, exists := store.Get("test123")
	if !exists {
		t.Fatal("Get should return true for existing link")
	}

	if retrievedLink.Code != link.Code {
		t.Errorf("Expected code %q, got %q", link.Code, retrievedLink.Code)
	}
	if retrievedLink.URL != link.URL {
		t.Errorf("Expected URL %q, got %q", link.URL, retrievedLink.URL)
	}
	if retrievedLink.Visits != link.Visits {
		t.Errorf("Expected visits %d, got %d", link.Visits, retrievedLink.Visits)
	}
}

func TestStore_CreateDuplicate(t *testing.T) {
	defer os.Remove("test3.json")
	defer os.Remove("test3.json.tmp")

	store, err := NewStore("test3.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	link := Link{
		Code:      "test123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Create first link
	err = store.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to create duplicate
	err = store.Create(link)
	if err == nil {
		t.Fatal("Create should fail for duplicate code")
	}
	if err.Error() != "code already exists" {
		t.Errorf("Expected 'code already exists', got %v", err)
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	defer os.Remove("test4.json")
	defer os.Remove("test4.json.tmp")

	store, err := NewStore("test4.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	link := Link{
		Code:      "test123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Create link
	err = store.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Increment visits
	err = store.IncrementVisits("test123")
	if err != nil {
		t.Fatalf("IncrementVisits failed: %v", err)
	}

	// Get link and check visits
	retrievedLink, exists := store.Get("test123")
	if !exists {
		t.Fatal("Get should return true for existing link")
	}

	if retrievedLink.Visits != 1 {
		t.Errorf("Expected visits 1, got %d", retrievedLink.Visits)
	}
}

func TestStore_IncrementVisitsNotFound(t *testing.T) {
	defer os.Remove("test5.json")
	defer os.Remove("test5.json.tmp")

	store, err := NewStore("test5.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Try to increment visits for non-existent link
	err = store.IncrementVisits("nonexistent")
	if err == nil {
		t.Fatal("IncrementVisits should fail for non-existent code")
	}
	if err.Error() != "code not found" {
		t.Errorf("Expected 'code not found', got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	defer os.Remove("test6.json")
	defer os.Remove("test6.json.tmp")

	store, err := NewStore("test6.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	link := Link{
		Code:      "test123",
		URL:       "https://example.com",
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Create link
	err = store.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete link
	err = store.Delete("test123")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify link is gone
	_, exists := store.Get("test123")
	if exists {
		t.Fatal("Get should return false for deleted link")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	defer os.Remove("test7.json")
	defer os.Remove("test7.json.tmp")

	store, err := NewStore("test7.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Try to delete non-existent link
	err = store.Delete("nonexistent")
	if err == nil {
		t.Fatal("Delete should fail for non-existent code")
	}
	if err.Error() != "code not found" {
		t.Errorf("Expected 'code not found', got %v", err)
	}
}

func TestStore_List(t *testing.T) {
	defer os.Remove("test8.json")
	defer os.Remove("test8.json.tmp")

	store, err := NewStore("test8.json")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create multiple links
	links := []Link{
		{
			Code:      "link1",
			URL:       "https://example.com/1",
			CreatedAt: time.Now().UTC(),
			Visits:    0,
		},
		{
			Code:      "link2",
			URL:       "https://example.com/2",
			CreatedAt: time.Now().UTC().Add(-time.Hour), // Older link
			Visits:    5,
		},
	}

	for _, link := range links {
		err := store.Create(link)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// List links
	list := store.List()

	// Should have 2 links
	if len(list) != 2 {
		t.Errorf("Expected 2 links, got %d", len(list))
	}

	// Should be sorted by creation time (oldest first)
	if list[0].Code != "link2" {
		t.Errorf("Expected oldest link first (link2), got %s", list[0].Code)
	}
	if list[1].Code != "link1" {
		t.Errorf("Expected newest link second (link1), got %s", list[1].Code)
	}
}

func (s *Store) Close() {
	// Clean up test files
	os.Remove(s.path)
	os.Remove(s.path + ".tmp")
}