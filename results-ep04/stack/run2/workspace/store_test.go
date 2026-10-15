package main

import (
	"sync"
	"testing"
	"time"
)

func TestMemoryStore_CreateAndGet(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	link := &Link{
		Code:      "abc123",
		URL:       "https://example.com",
		CreatedAt: time.Now(),
		Visits:    0,
	}

	// Test create
	err := store.Create(link)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Test get
	got, err := store.Get("abc123")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.URL != link.URL {
		t.Errorf("Get() got URL = %v, want %v", got.URL, link.URL)
	}

	// Test duplicate create
	err = store.Create(link)
	if err == nil {
		t.Error("Expected error on duplicate create, got nil")
	}
}

func TestMemoryStore_IncrementVisitsConcurrent(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	link := &Link{
		Code:      "test",
		URL:       "https://example.com",
		CreatedAt: time.Now(),
		Visits:    0,
	}
	store.Create(link)

	// Concurrent increments
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.IncrementVisits("test")
		}()
	}
	wg.Wait()

	got, _ := store.Get("test")
	if got.Visits != 100 {
		t.Errorf("After 100 concurrent increments, Visits = %d, want 100", got.Visits)
	}
}