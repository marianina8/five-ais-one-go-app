package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestEdgeCases_AdminAuth(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "shortener-edge-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "correct-token")

	tests := []struct {
		name        string
		authHeader  string
		wantStatus  int
		description string
	}{
		{
			name:        "no auth header",
			authHeader:  "",
			wantStatus:  http.StatusUnauthorized,
			description: "should require auth",
		},
		{
			name:        "wrong token",
			authHeader:  "Bearer wrong-token",
			wantStatus:  http.StatusUnauthorized,
			description: "should reject wrong token",
		},
		{
			name:        "wrong format",
			authHeader:  "Token correct-token",
			wantStatus:  http.StatusUnauthorized,
			description: "should require Bearer scheme",
		},
		{
			name:        "correct token",
			authHeader:  "Bearer correct-token",
			wantStatus:  http.StatusOK,
			description: "should accept correct token",
		},
		{
			name:        "extra spaces",
			authHeader:  "Bearer  correct-token",
			wantStatus:  http.StatusUnauthorized,
			description: "should reject extra spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/links", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("%s: status = %d, want %d", tt.description, rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestEdgeCases_URLValidation(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "shortener-url-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	tests := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{
			name:       "http URL",
			url:        "http://example.com",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "https URL",
			url:        "https://example.com",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "ftp URL",
			url:        "ftp://example.com",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "no scheme",
			url:        "example.com",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty URL",
			url:        "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "URL with port",
			url:        "https://example.com:8080/path",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "URL with query",
			url:        "https://example.com?query=value",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "URL with fragment",
			url:        "https://example.com#section",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "URL without host",
			url:        "https://",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "localhost URL",
			url:        "http://localhost:3000",
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"url": "` + tt.url + `"}`
			req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("URL %s: status = %d, want %d", tt.url, rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestEdgeCases_AliasValidation(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "shortener-alias-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewJSONStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := NewHandler(store, "test-token")

	tests := []struct {
		name       string
		alias      string
		wantStatus int
	}{
		{
			name:       "valid 3 chars",
			alias:      "abc",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid 32 chars",
			alias:      "abcdefghijklmnopqrstuvwxyz123456",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "too short (2 chars)",
			alias:      "ab",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "too long (33 chars)",
			alias:      "abcdefghijklmnopqrstuvwxyz1234567",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "api lowercase",
			alias:      "api",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "API uppercase",
			alias:      "API",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Api mixed case",
			alias:      "Api",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "with underscore",
			alias:      "my_page",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "with dash",
			alias:      "my-page",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "with numbers",
			alias:      "page123",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid char @",
			alias:      "page@123",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid char space",
			alias:      "page 123",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid char dot",
			alias:      "page.123",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"url": "https://example.com", "alias": "` + tt.alias + `"}`
			req := httptest.NewRequest("POST", "/api/links", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("Alias %s: status = %d, want %d", tt.alias, rr.Code, tt.wantStatus)
			}
		})
	}
}