package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLargeBody(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir + "/data.json")
	router := NewRouter(store, "secret")
	server := httptest.NewServer(router)
	defer server.Close()

	largeAlias := string(bytes.Repeat([]byte("a"), 1<<20))
	body := []byte(`{"url": "http://example.com", "alias": "` + largeAlias + `"}`)
	res, _ := http.Post(server.URL+"/api/links", "application/json", bytes.NewReader(body))
	
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", res.StatusCode)
	}
}
