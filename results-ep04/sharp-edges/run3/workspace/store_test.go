package main

import (
	"os"
	"sort"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) (*JSONStore, func()) {
	tmpFile, err := os.CreateTemp("", "shortener-test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return store, cleanup
}

func TestJSONStore_CreateAndGet(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	link, err := store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if link.Code != "test123" {
		t.Errorf("Expected code 'test123', got %s", link.Code)
	}

	if link.URL != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got %s", link.URL)
	}

	if link.Visits != 0 {
		t.Errorf("Expected visits 0, got %d", link.Visits)
	}

	// Test get
	retrieved, err := store.Get("test123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Code != link.Code {
		t.Errorf("Retrieved code mismatch: expected %s, got %s", link.Code, retrieved.Code)
	}
}

func TestJSONStore_DuplicateCode(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	_, err = store.Create("https://example.org", "test123")
	if err == nil {
		t.Fatal("Expected error for duplicate code")
	}
}

func TestJSONStore_IncrementVisits(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Increment multiple times
	for i := 0; i < 3; i++ {
		err = store.IncrementVisits("test123")
		if err != nil {
			t.Fatalf("IncrementVisits failed: %v", err)
		}
	}

	link, err := store.Get("test123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if link.Visits != 3 {
		t.Errorf("Expected 3 visits, got %d", link.Visits)
	}
}

func TestJSONStore_List(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create multiple links
	links := []struct {
		url  string
		code string
	}{
		{"https://example1.com", "code1"},
		{"https://example2.com", "code2"},
		{"https://example3.com", "code3"},
	}

	for _, l := range links {
		_, err := store.Create(l.url, l.code)
		if err != nil {
			t.Fatalf("Create %s failed: %v", l.code, err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(time.Millisecond)
	}

	list := store.List()
	if len(list) != 3 {
		t.Fatalf("Expected 3 links, got %d", len(list))
	}

	// Collect codes from list
	gotCodes := make([]string, 0, len(list))
	for _, link := range list {
		gotCodes = append(gotCodes, link.Code)
	}

	// Sort both slices for comparison
	sort.Strings(gotCodes)
	expectedCodes := []string{"code1", "code2", "code3"}
	sort.Strings(expectedCodes)

	for i := range expectedCodes {
		if gotCodes[i] != expectedCodes[i] {
			t.Errorf("Expected codes %v, got %v", expectedCodes, gotCodes)
			break
		}
	}
}

func TestJSONStore_Delete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	_, err := store.Create("https://example.com", "test123")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Should exist
	_, err = store.Get("test123")
	if err != nil {
		t.Fatalf("Get failed before delete: %v", err)
	}

	// Delete
	err = store.Delete("test123")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should not exist
	_, err = store.Get("test123")
	if err == nil {
		t.Fatal("Expected error after delete")
	}
}

func TestJSONStore_Persistence(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "shortener-persistence-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Create store and add data
	store1, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	_, err = store1.Create("https://example.com", "test123")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	store1.Close()

	// Create new store and verify data persists
	store2, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	link, err := store2.Get("test123")
	if err != nil {
		t.Fatalf("Get after reload failed: %v", err)
	}

	if link.URL != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got %s", link.URL)
	}
}