package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"shortener/internal/store"
)

func TestCreateLink(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	h := NewHandler(s, "test-token")
	
	reqBody := map[string]string{
		"url":   "https://example.com",
		"alias": "test-alias",
	}
	jsonReq, _ := json.Marshal(reqBody)
	
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/links", bytes.NewReader(jsonReq))
	
	h.CreateLink(rr, r)
	if rr.Code != http.StatusCreated {
		t.Errorf("Expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	
	if resp["code"] != "test-alias" {
		t.Errorf("Expected code 'test-alias', got %v", resp["code"])
	}
	if resp["url"] != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got %v", resp["url"])
	}
	if resp["visits"] != 0.0 {
		t.Errorf("Expected visits 0, got %v", resp["visits"])
	}
}

func TestCreateLinkValidation(t *testing.T) {
	s := store.NewJsonStore(t.TempDir() + "/data.json")
	h := NewHandler(s, "test-token")
	
	tests := []struct {
		name   string
		body   map[string]string
		status int
	}{
		{"invalid URL", map[string]string{"url": "ftp://example.com"}, http.StatusBadRequest},
		{"missing URL", map[string]string{"alias": "test"}, http.StatusBadRequest},
		{"invalid alias short", map[string]string{"url": "https://example.com", "alias": "ab"}, http.StatusBadRequest},
		{"invalid alias chars", map[string]string{"url": "https://example.com", "alias": "test!"}, http.StatusBadRequest},
		{"alias 'api'", map[string]string{"url": "https://example.com", "alias": "api"}, http.StatusBadRequest},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, _ := json.Marshal(tt.body)
			rr := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/links", bytes.NewReader(jsonBody))
			
			h.CreateLink(rr, r)
			if rr.Code != tt.status {
				t.Errorf("Expected status %d, got %d: %s", tt.status, rr.Code, rr.Body.String())
			}
		})
	}
}