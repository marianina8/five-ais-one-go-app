package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url    string
		valid  bool
	}{
		{"https://example.com", true},
		{"http://localhost:8080", true},
		{"ftp://example.com", false},
		{"example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		got := validateURL(tt.url)
		if got != tt.valid {
			t.Errorf("validateURL(%q) = %v, want %v", tt.url, got, tt.valid)
		}
	}
}

func TestValidateAlias(t *testing.T) {
	tests := []struct {
		alias  string
		valid  bool
	}{
		{"my-alias_123", true},
		{"api", false},
		{"ab", false},
		{"too-long-alias-that-is-more-than-32-chars", false},
		{"", true}, // empty is valid (means generate random)
		{"invalid!char", false},
	}
	for _, tt := range tests {
		got := validateAlias(tt.alias)
		if got != tt.valid {
			t.Errorf("validateAlias(%q) = %v, want %v", tt.alias, got, tt.valid)
		}
	}
}

func TestAdminAuth(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	authHandler := adminAuth("secret-token", handler)
	
	// Test missing auth
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	authHandler.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for missing auth, got %d", rr.Code)
	}
	
	// Test invalid token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rr = httptest.NewRecorder()
	authHandler.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for wrong token, got %d", rr.Code)
	}
	
	// Test valid token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rr = httptest.NewRecorder()
	authHandler.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 for valid token, got %d", rr.Code)
	}
	
	// Test malformed auth header
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "NotBearer token")
	rr = httptest.NewRecorder()
	authHandler.ServeHTTP(rr, req)
	
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for malformed header, got %d", rr.Code)
	}
}