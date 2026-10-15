package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestJSONStore_CreateLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Test creating a link with auto-generated code
	link1, err := store.CreateLink("https://example.com", "")
	if err != nil {
		t.Errorf("CreateLink failed: %v", err)
	}
	if link1.Code == "" {
		t.Error("Expected code to be generated")
	}
	if link1.URL != "https://example.com" {
		t.Errorf("Expected URL to be https://example.com, got %s", link1.URL)
	}
	if link1.Visits != 0 {
		t.Errorf("Expected visits to be 0, got %d", link1.Visits)
	}

	// Test creating a link with alias
	link2, err := store.CreateLink("https://google.com", "google")
	if err != nil {
		t.Errorf("CreateLink with alias failed: %v", err)
	}
	if link2.Code != "google" {
		t.Errorf("Expected code to be 'google', got %s", link2.Code)
	}

	// Test duplicate alias
	_, err = store.CreateLink("https://example.org", "google")
	if err == nil {
		t.Error("Expected error for duplicate alias")
	}
	if err.Error() != "alias already taken" {
		t.Errorf("Expected 'alias already taken', got %v", err)
	}
}

func TestJSONStore_GetLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a link
	link, err := store.CreateLink("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Get the link
	retrieved, err := store.GetLink("test")
	if err != nil {
		t.Errorf("GetLink failed: %v", err)
	}
	if retrieved.Code != link.Code {
		t.Errorf("Expected code %s, got %s", link.Code, retrieved.Code)
	}
	if retrieved.URL != link.URL {
		t.Errorf("Expected URL %s, got %s", link.URL, retrieved.URL)
	}

	// Test getting non-existent link
	_, err = store.GetLink("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent link")
	}
	if err.Error() != "link not found" {
		t.Errorf("Expected 'link not found', got %v", err)
	}
}

func TestJSONStore_IncrementVisits(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a link
	_, err = store.CreateLink("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Increment visits
	for i := 0; i < 3; i++ {
		err = store.IncrementVisits("test")
		if err != nil {
			t.Errorf("IncrementVisits failed: %v", err)
		}
	}

	// Check visits count
	link, err := store.GetLink("test")
	if err != nil {
		t.Fatal(err)
	}
	if link.Visits != 3 {
		t.Errorf("Expected 3 visits, got %d", link.Visits)
	}

	// Test incrementing non-existent link
	err = store.IncrementVisits("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent link")
	}
	if err.Error() != "link not found" {
		t.Errorf("Expected 'link not found', got %v", err)
	}
}

func TestJSONStore_ListLinks(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create multiple links with delays to ensure different timestamps
	links := []struct {
		url   string
		alias string
	}{
		{"https://example.com/1", "link1"},
		{"https://example.com/2", "link2"},
		{"https://example.com/3", "link3"},
	}

	for _, l := range links {
		time.Sleep(time.Millisecond) // Ensure different timestamps
		_, err := store.CreateLink(l.url, l.alias)
		if err != nil {
			t.Fatal(err)
		}
	}

	// List links
	list, err := store.ListLinks()
	if err != nil {
		t.Errorf("ListLinks failed: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("Expected 3 links, got %d", len(list))
	}

	// Check ordering (oldest first)
	for i := 0; i < len(list)-1; i++ {
		if list[i].CreatedAt.After(list[i+1].CreatedAt) {
			t.Error("Links not sorted by creation time (oldest first)")
		}
	}
}

func TestJSONStore_DeleteLink(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a link
	_, err = store.CreateLink("https://example.com", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Delete the link
	err = store.DeleteLink("test")
	if err != nil {
		t.Errorf("DeleteLink failed: %v", err)
	}

	// Verify link is deleted
	_, err = store.GetLink("test")
	if err == nil {
		t.Error("Expected error after deletion")
	}
	if err.Error() != "link not found" {
		t.Errorf("Expected 'link not found', got %v", err)
	}

	// Test deleting non-existent link
	err = store.DeleteLink("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent link")
	}
	if err.Error() != "link not found" {
		t.Errorf("Expected 'link not found', got %v", err)
	}
}

func TestJSONStore_URLValidation(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Test invalid URLs
	invalidURLs := []struct {
		url  string
		desc string
	}{
		{"example.com", "missing scheme"},
		{"ftp://example.com", "invalid scheme"},
		{"http://", "no host"},
		{"", "empty URL"},
	}

	for _, tc := range invalidURLs {
		_, err := store.CreateLink(tc.url, "")
		if err == nil {
			t.Errorf("Expected error for %s", tc.desc)
		}
	}

	// Test valid URLs
	validURLs := []string{
		"https://example.com",
		"http://example.com/path",
		"https://sub.example.com:8080/path?query=value",
	}

	for _, url := range validURLs {
		_, err := store.CreateLink(url, "")
		if err != nil {
			t.Errorf("Valid URL %q rejected: %v", url, err)
		}
	}
}

func TestJSONStore_AliasValidation(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	store, err := NewJSONStore(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Test invalid aliases
	invalidAliases := []struct {
		alias string
		desc  string
	}{
		{"ab", "too short"},
		{strings.Repeat("a", 33), "too long"},
		{"api", "reserved word"},
		{"test@test", "invalid characters"},
		{"test space", "invalid characters"},
	}

	for _, tc := range invalidAliases {
		_, err := store.CreateLink("https://example.com", tc.alias)
		if err == nil {
			t.Errorf("Expected error for alias %s (%s)", tc.alias, tc.desc)
		} else if err.Error() != "invalid alias" {
			t.Errorf("Expected 'invalid alias' for %s, got %v", tc.desc, err)
		}
	}

	// Test valid aliases
	validAliases := []string{
		"abc",
		"test-123",
		"test_456",
		"A-Za-z0-9_-32", // 32 chars
	}

	for _, alias := range validAliases {
		_, err := store.CreateLink("https://example.com", alias)
		if err != nil {
			t.Errorf("Valid alias %q rejected: %v", alias, err)
		}
	}
}