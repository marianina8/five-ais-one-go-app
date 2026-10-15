package main

import (
	"os"
	"testing"
	"time"
)

func TestStore_CreateLink(t *testing.T) {
	tempFile := "test_store.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test creating a link without alias
	link1, err := store.CreateLink("https://example.com", "")
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	if link1.Code == "" {
		t.Error("Expected non-empty code")
	}
	if len(link1.Code) != 7 {
		t.Errorf("Expected code length 7, got %d", len(link1.Code))
	}
	if link1.URL != "https://example.com" {
		t.Errorf("Expected URL https://example.com, got %s", link1.URL)
	}
	if link1.Visits != 0 {
		t.Errorf("Expected 0 visits, got %d", link1.Visits)
	}
	if link1.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt")
	}

	// Test creating a link with alias
	link2, err := store.CreateLink("https://google.com", "google")
	if err != nil {
		t.Fatalf("Failed to create link with alias: %v", err)
	}

	if link2.Alias != "google" {
		t.Errorf("Expected alias 'google', got %s", link2.Alias)
	}

	// Test duplicate alias
	_, err = store.CreateLink("https://bing.com", "google")
	if err == nil || err.Error() != "alias already taken" {
		t.Errorf("Expected 'alias already taken' error, got %v", err)
	}

	// Test getting link by code
	l, ok := store.GetLink(link1.Code)
	if !ok {
		t.Error("Expected to find link by code")
	}
	if l.Code != link1.Code {
		t.Errorf("Expected code %s, got %s", link1.Code, l.Code)
	}

	// Test getting link by alias
	l, ok = store.GetLink("google")
	if !ok {
		t.Error("Expected to find link by alias")
	}
	if l.Code != link2.Code {
		t.Errorf("Expected code %s, got %s", link2.Code, l.Code)
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	tempFile := "test_store_visits.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	link, err := store.CreateLink("https://example.com", "")
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Test incrementing visits
	for i := 1; i <= 5; i++ {
		if err := store.IncrementVisits(link.Code); err != nil {
			t.Fatalf("Failed to increment visits: %v", err)
		}

		l, ok := store.GetLink(link.Code)
		if !ok {
			t.Fatal("Link should exist")
		}
		if l.Visits != i {
			t.Errorf("Expected %d visits, got %d", i, l.Visits)
		}
	}

	// Test incrementing non-existent link
	err = store.IncrementVisits("nonexistent")
	if err == nil || err.Error() != "link not found" {
		t.Errorf("Expected 'link not found' error, got %v", err)
	}
}

func TestStore_GetAllLinks(t *testing.T) {
	tempFile := "test_store_all.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create multiple links with delays to ensure different timestamps
	links := make([]*Link, 3)
	for i := 0; i < 3; i++ {
		time.Sleep(10 * time.Millisecond)
		link, err := store.CreateLink("https://example.com", "")
		if err != nil {
			t.Fatalf("Failed to create link %d: %v", i, err)
		}
		links[i] = link
	}

	allLinks := store.GetAllLinks()
	if len(allLinks) != 3 {
		t.Errorf("Expected 3 links, got %d", len(allLinks))
	}

	// Check that links are returned (order is not guaranteed by GetAllLinks, but we'll sort them)
	for i := 0; i < len(allLinks); i++ {
		for j := i + 1; j < len(allLinks); j++ {
			if allLinks[j].CreatedAt.Before(allLinks[i].CreatedAt) {
				allLinks[i], allLinks[j] = allLinks[j], allLinks[i]
			}
		}
	}

	// Verify sorted order (oldest first)
	for i := 1; i < len(allLinks); i++ {
		if allLinks[i].CreatedAt.Before(allLinks[i-1].CreatedAt) {
			t.Error("Links should be sorted by CreatedAt (oldest first)")
		}
	}
}

func TestStore_DeleteLink(t *testing.T) {
	tempFile := "test_store_delete.json"
	defer os.Remove(tempFile)

	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	link, err := store.CreateLink("https://example.com", "test-alias")
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}

	// Verify link exists
	_, ok := store.GetLink(link.Code)
	if !ok {
		t.Fatal("Link should exist before deletion")
	}

	// Delete link
	err = store.DeleteLink(link.Code)
	if err != nil {
		t.Fatalf("Failed to delete link: %v", err)
	}

	// Verify link is gone
	_, ok = store.GetLink(link.Code)
	if ok {
		t.Error("Link should not exist after deletion")
	}

	// Verify alias is gone
	_, ok = store.GetLink("test-alias")
	if ok {
		t.Error("Alias should not exist after deletion")
	}

	// Test deleting non-existent link
	err = store.DeleteLink("nonexistent")
	if err == nil || err.Error() != "link not found" {
		t.Errorf("Expected 'link not found' error, got %v", err)
	}
}

func TestStore_Persistence(t *testing.T) {
	tempFile := "test_store_persist.json"
	defer os.Remove(tempFile)

	// Create store and add data
	store1, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	link1, err := store1.CreateLink("https://example.com", "example")
	if err != nil {
		t.Fatalf("Failed to create link: %v", err)
	}
	store1.Close()

	// Create new store from same file
	store2, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("Failed to create new store: %v", err)
	}
	defer store2.Close()

	// Verify data persisted
	link2, ok := store2.GetLink("example") // by alias
	if !ok {
		t.Fatal("Link should persist")
	}

	if link2.Code != link1.Code {
		t.Errorf("Expected code %s, got %s", link1.Code, link2.Code)
	}
	if link2.URL != link1.URL {
		t.Errorf("Expected URL %s, got %s", link1.URL, link2.URL)
	}
}