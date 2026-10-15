package store

import (
	"os"
	"testing"
)

func TestJsonStore_CreateGet(t *testing.T) {
	s := NewJsonStore(t.TempDir() + "/data.json")
	defer os.Remove(t.TempDir() + "/data.json")
	
	link, err := s.Create("https://example.com", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if link.URL != "https://example.com" {
		t.Errorf("Expected URL %q, got %q", "https://example.com", link.URL)
	}
	if len(link.Code) != 7 {
		t.Errorf("Expected code length 7, got %d", len(link.Code))
	}
	
	link2, err := s.Get(link.Code)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if link2.URL != link.URL {
		t.Errorf("Expected URL %q, got %q", link.URL, link2.URL)
	}
}