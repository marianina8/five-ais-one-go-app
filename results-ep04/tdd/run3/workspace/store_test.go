package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestStore_AddWithAlias(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	link, err := store.Add("https://example.com", "test-alias")
	if err != nil {
		t.Fatalf("failed to add link: %v", err)
	}

	if link.Code != "test-alias" {
		t.Errorf("expected code 'test-alias', got %s", link.Code)
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link.Visits)
	}

	// Verify duplicate alias returns conflict error
	_, err = store.Add("https://example2.com", "test-alias")
	if err == nil {
		t.Error("expected error for duplicate alias")
	}
	if _, ok := err.(*ConflictError); !ok {
		t.Errorf("expected ConflictError, got %T", err)
	}
}

func TestStore_AddRandomCode(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	link, err := store.Add("https://example.com", "")
	if err != nil {
		t.Fatalf("failed to add link: %v", err)
	}

	if len(link.Code) != 7 {
		t.Errorf("expected code length 7, got %d", len(link.Code))
	}
}

func TestStore_Get(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Add("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	link, err := store.Get("test")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}
	if link.Code != "test" {
		t.Errorf("expected code 'test', got %s", link.Code)
	}

	_, err = store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent code")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Errorf("expected NotFoundError, got %T", err)
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Add("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	link, _ := store.Get("test")
	if link.Visits != 0 {
		t.Errorf("expected 0 visits initially, got %d", link.Visits)
	}

	err = store.IncrementVisits("test")
	if err != nil {
		t.Fatalf("failed to increment visits: %v", err)
	}

	link, _ = store.Get("test")
	if link.Visits != 1 {
		t.Errorf("expected 1 visit, got %d", link.Visits)
	}

	err = store.IncrementVisits("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent code")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Errorf("expected NotFoundError, got %T", err)
	}
}

func TestStore_Delete(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Add("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	err = store.Delete("test")
	if err != nil {
		t.Fatalf("failed to delete link: %v", err)
	}

	_, err = store.Get("test")
	if err == nil {
		t.Error("expected error after deletion")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Errorf("expected NotFoundError, got %T", err)
	}

	err = store.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent code")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Errorf("expected NotFoundError, got %T", err)
	}
}

func TestStore_List(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	store, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Add links in reverse order
	_, err = store.Add("https://example2.com", "test2")
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Add("https://example1.com", "test1")
	if err != nil {
		t.Fatal(err)
	}

	links := store.List()
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}

	// Should be sorted by creation time
	if links[0].Code != "test2" {
		t.Errorf("expected first link to be 'test2', got %s", links[0].Code)
	}
	if links[1].Code != "test1" {
		t.Errorf("expected second link to be 'test1', got %s", links[1].Code)
	}
}

func TestStore_Persistence(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test-store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	// Create store and add link
	store1, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store1.Add("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Create new store instance to load from file
	store2, err := NewStore(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	link, err := store2.Get("test")
	if err != nil {
		t.Fatalf("failed to load persisted link: %v", err)
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", link.URL)
	}

	// Verify file contains valid JSON
	data, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to read data file: %v", err)
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(links) != 1 {
		t.Errorf("expected 1 link in JSON, got %d", len(links))
	}
}