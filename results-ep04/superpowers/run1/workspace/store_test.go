package main

import (
	"path/filepath"
	"testing"
	"time"
)

func createTestStore(t *testing.T) *FileStore {
	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "data.json")
	
	s, err := newFileStore(dataFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	return s
}

func TestStore_CreateLinkWithAlias(t *testing.T) {
	s := createTestStore(t)
	defer s.Close()

	link := Link{
		Code:      "my-page",
		URL:       "https://example.com/page",
		CreatedAt: time.Now(),
		Visits:    0,
	}

	err := s.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Retrieve the link
	retrieved, err := s.Get("my-page")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Code != "my-page" {
		t.Errorf("expected code 'my-page', got %q", retrieved.Code)
	}
	if retrieved.URL != "https://example.com/page" {
		t.Errorf("expected URL 'https://example.com/page', got %q", retrieved.URL)
	}
	if retrieved.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", retrieved.Visits)
	}
}

func TestStore_CreateLinkDuplicateAlias(t *testing.T) {
	s := createTestStore(t)
	defer s.Close()

	link1 := Link{
		Code: "my-page",
		URL:  "https://example.com/page1",
		CreatedAt: time.Now(),
	}

	err := s.Create(link1)
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	link2 := Link{
		Code: "my-page",
		URL:  "https://example.com/page2",
		CreatedAt: time.Now(),
	}

	err = s.Create(link2)
	if err == nil {
		t.Fatal("expected error for duplicate alias, got nil")
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	s := createTestStore(t)
	defer s.Close()

	link := Link{
		Code: "test",
		URL:  "https://example.com",
		CreatedAt: time.Now(),
		Visits: 0,
	}

	err := s.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Increment visits
	err = s.IncrementVisits("test")
	if err != nil {
		t.Fatalf("IncrementVisits failed: %v", err)
	}

	retrieved, err := s.Get("test")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Visits != 1 {
		t.Errorf("expected 1 visit after increment, got %d", retrieved.Visits)
	}
}

func TestStore_DeleteLink(t *testing.T) {
	s := createTestStore(t)
	defer s.Close()

	link := Link{
		Code: "test",
		URL:  "https://example.com",
		CreatedAt: time.Now(),
		Visits: 0,
	}

	err := s.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete the link
	err = s.Delete("test")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should not find the link
	_, err = s.Get("test")
	if err == nil {
		t.Fatal("expected error for deleted link, got nil")
	}
}

func TestStore_ListLinks(t *testing.T) {
	s := createTestStore(t)
	defer s.Close()

	link1 := Link{
		Code:      "first",
		URL:       "https://example.com/1",
		CreatedAt: time.Now().Add(-time.Hour),
		Visits:    0,
	}

	link2 := Link{
		Code:      "second",
		URL:       "https://example.com/2",
		CreatedAt: time.Now(),
		Visits:    0,
	}

	err := s.Create(link1)
	if err != nil {
		t.Fatalf("Create link1 failed: %v", err)
	}

	err = s.Create(link2)
	if err != nil {
		t.Fatalf("Create link2 failed: %v", err)
	}

	links := s.List()
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}

	// Should be sorted by CreatedAt ascending (oldest first)
	if links[0].Code != "first" {
		t.Errorf("expected first link to be 'first', got %q", links[0].Code)
	}
	if links[1].Code != "second" {
		t.Errorf("expected second link to be 'second', got %q", links[1].Code)
	}
}