package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_CreateAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test.json"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	link, err := store.Create("test123", "https://example.com")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	if link.Code != "test123" {
		t.Errorf("expected code 'test123', got %s", link.Code)
	}
	if link.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %s", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("expected 0 visits, got %d", link.Visits)
	}

	retrieved, err := store.Get("test123")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}

	if retrieved.Code != link.Code || retrieved.URL != link.URL {
		t.Errorf("retrieved link doesn't match created link")
	}
}

func TestStore_DuplicateCode(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test.json"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.Create("test123", "https://example.com")
	if err != nil {
		t.Fatalf("failed to create first link: %v", err)
	}

	_, err = store.Create("test123", "https://example.org")
	if err == nil || err.Error() != "code already exists" {
		t.Errorf("expected 'code already exists' error, got %v", err)
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test.json"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	link, err := store.Create("test123", "https://example.com")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	if link.Visits != 0 {
		t.Errorf("expected 0 initial visits, got %d", link.Visits)
	}

	err = store.IncrementVisits("test123")
	if err != nil {
		t.Fatalf("failed to increment visits: %v", err)
	}

	updated, err := store.Get("test123")
	if err != nil {
		t.Fatalf("failed to get link: %v", err)
	}

	if updated.Visits != 1 {
		t.Errorf("expected 1 visit after increment, got %d", updated.Visits)
	}
}

func TestStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test.json"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.Create("test123", "https://example.com")
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	_, err = store.Get("test123")
	if err != nil {
		t.Fatalf("link should exist: %v", err)
	}

	err = store.Delete("test123")
	if err != nil {
		t.Fatalf("failed to delete link: %v", err)
	}

	_, err = store.Get("test123")
	if err == nil || err.Error() != "link not found" {
		t.Errorf("expected 'link not found' error after deletion, got %v", err)
	}
}

func TestStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(filepath.Join(tmpDir, "test.json"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	links := []struct {
		code string
		url  string
	}{
		{"code1", "https://example.com"},
		{"code2", "https://example.org"},
		{"code3", "https://example.net"},
	}

	for _, l := range links {
		_, err := store.Create(l.code, l.url)
		if err != nil {
			t.Fatalf("failed to create link %s: %v", l.code, err)
		}
	}

	list := store.List()
	if len(list) != len(links) {
		t.Errorf("expected %d links in list, got %d", len(links), len(list))
	}

	found := make(map[string]bool)
	for _, link := range list {
		found[link.Code] = true
	}

	for _, l := range links {
		if !found[l.code] {
			t.Errorf("link %s not found in list", l.code)
		}
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "test.json")

	store1, err := NewStore(file)
	if err != nil {
		t.Fatalf("failed to create store1: %v", err)
	}

	_, err = store1.Create("test123", "https://example.com")
	if err != nil {
		t.Fatalf("failed to create link in store1: %v", err)
	}

	store1.Close()

	store2, err := NewStore(file)
	if err != nil {
		t.Fatalf("failed to create store2: %v", err)
	}
	defer store2.Close()

	link, err := store2.Get("test123")
	if err != nil {
		t.Fatalf("failed to get persisted link: %v", err)
	}

	if link.Code != "test123" || link.URL != "https://example.com" {
		t.Errorf("persisted link doesn't match original")
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("failed to read data file: %v", err)
	}

	var storedLinks []*Link
	if err := json.Unmarshal(data, &storedLinks); err != nil {
		t.Fatalf("failed to unmarshal stored data: %v", err)
	}

	if len(storedLinks) != 1 {
		t.Errorf("expected 1 link in file, got %d", len(storedLinks))
	}
}