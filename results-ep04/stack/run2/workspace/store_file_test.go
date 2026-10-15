package main

import (
	"os"
	"testing"
	"time"
)

func TestFileStore_Persistence(t *testing.T) {
	tempFile, err := os.CreateTemp("", "shortener-test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Create store, add data
	store1, err := NewFileStore(tempFile.Name())
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	link := &Link{
		Code:      "persist",
		URL:       "https://example.com",
		CreatedAt: time.Now(),
		Visits:    5,
	}
	store1.Create(link)
	store1.IncrementVisits("persist") // Should trigger save
	store1.Close()

	// Load from same file
	store2, err := LoadFromFile(tempFile.Name())
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}
	defer store2.Close()

	got, err := store2.Get("persist")
	if err != nil {
		t.Fatalf("Get after reload failed: %v", err)
	}
	if got.Visits != 6 {
		t.Errorf("Persisted Visits = %d, want 6", got.Visits)
	}
}