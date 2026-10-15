package main

import (
	"strings"
	"testing"
)

func TestExampleUsage(t *testing.T) {
	// This test demonstrates the complete flow of the URL shortener
	
	// Create a test server
	tempFile := t.TempDir() + "/data.json"
	store, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// 1. Create links
	link1, err := store.Create("https://example.com", "")
	if err != nil {
		t.Fatalf("failed to create link1: %v", err)
	}
	
	if len(link1.Code) != 7 {
		t.Errorf("expected random code length 7, got %d", len(link1.Code))
	}
	
	link2, err := store.Create("https://google.com", "search")
	if err != nil {
		t.Fatalf("failed to create link2: %v", err)
	}
	
	if link2.Code != "search" {
		t.Errorf("expected code 'search', got %s", link2.Code)
	}
	
	// 2. Follow links (increment visits)
	for i := 0; i < 3; i++ {
		_, err = store.Get("search")
		if err != nil {
			t.Fatalf("failed to follow link on iteration %d: %v", i, err)
		}
	}
	
	// 3. Verify visit count
	linkAfterVisits, err := store.Get("search")
	if err != nil {
		t.Fatalf("failed to get link after visits: %v", err)
	}
	
	// Should have 4 visits now (3 above + this one)
	if linkAfterVisits.Visits != 4 {
		t.Errorf("expected 4 visits, got %d", linkAfterVisits.Visits)
	}
	
	// 4. List all links
	allLinks := store.GetAll()
	if len(allLinks) != 2 {
		t.Errorf("expected 2 links, got %d", len(allLinks))
	}
	
	// 5. Delete a link
	err = store.Delete("search")
	if err != nil {
		t.Fatalf("failed to delete link: %v", err)
	}
	
	// 6. Verify deletion
	_, err = store.Get("search")
	if err == nil {
		t.Error("expected error after deletion, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "link not found") {
		t.Errorf("expected 'link not found' error, got %v", err)
	}
	
	// 7. Verify remaining links
	finalLinks := store.GetAll()
	if len(finalLinks) != 1 {
		t.Errorf("expected 1 remaining link, got %d", len(finalLinks))
	}
	
	// 8. Test persistence by creating new store from same file
	store2, err := NewStore(tempFile)
	if err != nil {
		t.Fatalf("failed to create new store from file: %v", err)
	}
	
	persistedLinks := store2.GetAll()
	if len(persistedLinks) != 1 {
		t.Errorf("expected 1 persisted link, got %d", len(persistedLinks))
	}
	
	// Verify the persisted link is correct
	persistedLink, err := store2.Get(link1.Code)
	if err != nil {
		t.Fatalf("failed to get persisted link: %v", err)
	}
	
	if persistedLink.URL != "https://example.com" {
		t.Errorf("expected persisted URL https://example.com, got %s", persistedLink.URL)
	}
}