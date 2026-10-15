package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPathMatching(t *testing.T) {
	store, cleanup := createTestStore(t)
	defer cleanup()
	
	s := NewServer(store, "test-token")

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{"GET", "/api/links", http.StatusUnauthorized}, // No auth
		{"POST", "/api/links", http.StatusBadRequest},  // Missing body
		{"DELETE", "/api/links/test", http.StatusUnauthorized}, // No auth
		{"GET", "/test", http.StatusNotFound}, // Link doesn't exist
		{"GET", "/", http.StatusNotFound}, // Root path
		{"GET", "/api", http.StatusNotFound}, // /api without /links
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			
			s.ServeHTTP(rr, req)
			
			if rr.Code != tt.want {
				t.Errorf("got status %d, want %d for %s %s", rr.Code, tt.want, tt.method, tt.path)
			}
		})
	}
}