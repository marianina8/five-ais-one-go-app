package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStore_CreateGetDelete(t *testing.T) {
	// Create temporary file
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Test create without alias
	link, err := store.Create("https://example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if link.Code == "" {
		t.Error("Expected generated code")
	}
	if link.URL != "https://example.com" {
		t.Errorf("Expected URL https://example.com, got %s", link.URL)
	}
	if link.Visits != 0 {
		t.Errorf("Expected 0 visits, got %d", link.Visits)
	}

	// Test get
	link2, err := store.Get(link.Code)
	if err != nil {
		t.Fatal(err)
	}
	if link2.Code != link.Code {
		t.Errorf("Expected code %s, got %s", link.Code, link2.Code)
	}

	// Test create with alias
	link3, err := store.Create("https://example2.com", "test-alias")
	if err != nil {
		t.Fatal(err)
	}
	if link3.Code != "test-alias" {
		t.Errorf("Expected code test-alias, got %s", link3.Code)
	}

	// Test duplicate alias
	_, err = store.Create("https://example3.com", "test-alias")
	if err != ErrAliasTaken {
		t.Errorf("Expected ErrAliasTaken, got %v", err)
	}

	// Test list
	links, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Errorf("Expected 2 links, got %d", len(links))
	}

	// Test delete
	err = store.Delete(link.Code)
	if err != nil {
		t.Fatal(err)
	}

	// Verify deleted
	_, err = store.Get(link.Code)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}

	// List should now have 1
	links, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Errorf("Expected 1 link after delete, got %d", len(links))
	}
}

func TestStore_IncrementVisits(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	link, err := store.Create("https://example.com", "test-visits")
	if err != nil {
		t.Fatal(err)
	}

	if link.Visits != 0 {
		t.Errorf("Expected 0 initial visits, got %d", link.Visits)
	}

	// Increment
	err = store.IncrementVisits("test-visits")
	if err != nil {
		t.Fatal(err)
	}

	// Get and verify
	link2, err := store.Get("test-visits")
	if err != nil {
		t.Fatal(err)
	}
	if link2.Visits != 1 {
		t.Errorf("Expected 1 visit after increment, got %d", link2.Visits)
	}

	// Increment again
	err = store.IncrementVisits("test-visits")
	if err != nil {
		t.Fatal(err)
	}

	link3, err := store.Get("test-visits")
	if err != nil {
		t.Fatal(err)
	}
	if link3.Visits != 2 {
		t.Errorf("Expected 2 visits after second increment, got %d", link3.Visits)
	}
}

func TestStore_InvalidURLs(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cases := []struct {
		url       string
		shouldErr bool
	}{
		{"ftp://example.com", true}, // wrong scheme
		{"http://", true},           // missing host
		{"https://example.com", false}, // valid
		{"http://example.com", false},  // valid
	}

	for _, tc := range cases {
		_, err := store.Create(tc.url, "")
		hasErr := err != nil && err == ErrInvalidURL
		if tc.shouldErr != hasErr {
			t.Errorf("URL %s: expected err=%v, got err=%v (err=%v)", tc.url, tc.shouldErr, hasErr, err)
		}
	}
}

func TestStore_InvalidAliases(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cases := []struct {
		alias     string
		shouldErr bool
	}{
		{"api", true},          // reserved
		{"ab", true},           // too short
		{"a" * 33, true},       // too long
		{"valid-alias", false}, // valid
		{"test_123", false},    // valid
		{"test@123", true},     // invalid char
		{"test 123", true},     // invalid char
	}

	for _, tc := range cases {
		_, err := store.Create("https://example.com", tc.alias)
		hasErr := err != nil && err == ErrInvalidAlias
		if tc.shouldErr != hasErr {
			t.Errorf("Alias %s: expected err=%v, got err=%v (err=%v)", tc.alias, tc.shouldErr, hasErr, err)
		}
	}
}

func TestStore_Concurrent(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create initial link
	link, err := store.Create("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Concurrent visits
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			store.IncrementVisits("test")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Verify count
	link2, err := store.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if link2.Visits != 101 { // 1 initial creation + 100 increments
		t.Errorf("Expected 101 visits, got %d", link2.Visits)
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "teststore*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Create first store and add data
	store1, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	link, err := store1.Create("https://example.com", "persistent-test")
	if err != nil {
		t.Fatal(err)
	}

	err = store1.IncrementVisits("persistent-test")
	if err != nil {
		t.Fatal(err)
	}

	store1.Close()

	// Create second store from same file
	store2, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	// Verify data persisted
	link2, err := store2.Get("persistent-test")
	if err != nil {
		t.Fatal(err)
	}
	if link2.URL != "https://example.com" {
		t.Errorf("Expected URL https://example.com, got %s", link2.URL)
	}
	if link2.Visits != 1 {
		t.Errorf("Expected 1 visit, got %d", link2.Visits)
	}
}