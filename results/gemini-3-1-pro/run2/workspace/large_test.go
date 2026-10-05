package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPayloadTooLarge(t *testing.T) {
	mux := http.NewServeMux()
	server := &Server{store: &Store{}}
	mux.HandleFunc("POST /api/links", server.handleCreateLink)
	ts := httptest.NewServer(wrapInterceptor(mux))
	defer ts.Close()

	largeBody := `{"url":"http://example.com","alias":"` + strings.Repeat("a", 1024*1024) + `"}`
	req, _ := http.NewRequest("POST", ts.URL+"/api/links", bytes.NewBufferString(largeBody))
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}
