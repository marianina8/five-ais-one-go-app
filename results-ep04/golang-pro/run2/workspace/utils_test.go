package main

import (
	"testing"
)

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		// Valid URLs
		{"Valid HTTPS", "https://example.com", true},
		{"Valid HTTP", "http://example.com", true},
		{"Valid with path", "https://example.com/path", true},
		{"Valid with query", "https://example.com?q=test", true},
		{"Valid with fragment", "https://example.com#section", true},
		
		// Invalid URLs
		{"Missing scheme", "example.com", false},
		{"Wrong scheme", "ftp://example.com", false},
		{"Missing host", "http://", false},
		{"Relative URL", "/path", false},
		{"Invalid URL", "://example.com", false},
		{"Too long", "http://" + string(make([]byte, 2049-11)), false}, // 11 for "http://"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidURL(tt.url)
			if result != tt.expected {
				t.Errorf("isValidURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestIsValidAlias(t *testing.T) {
	tests := []struct {
		name     string
		alias    string
		expected bool
	}{
		// Valid aliases
		{"Valid short", "abc", true},
		{"Valid with numbers", "abc123", true},
		{"Valid with dash", "abc-def", true},
		{"Valid with underscore", "abc_def", true},
		{"Valid mixed", "a1-b2_c3", true},
		{"Max length", "abcdefghijklmnopqrstuvwxyz012345", true}, // 32 chars
		
		// Invalid aliases
		{"Too short", "ab", false},
		{"Too long", "abcdefghijklmnopqrstuvwxyz0123456", false}, // 33 chars
		{"Empty", "", false},
		{"Invalid chars", "abc@def", false},
		{"Invalid chars", "abc.def", false},
		{"Invalid chars", "abc def", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidAlias(tt.alias)
			if result != tt.expected {
				t.Errorf("isValidAlias(%q) = %v, want %v", tt.alias, result, tt.expected)
			}
		})
	}
}

func TestGenerateRandomCode(t *testing.T) {
	tests := []struct {
		length int
	}{
		{1},
		{7},
		{10},
		{32},
		{0},
		{-1},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			code := generateRandomCode(tt.length)
			if tt.length <= 0 && code != "" {
				t.Errorf("generateRandomCode(%d) = %q, want empty string", tt.length, code)
			}
			if tt.length > 0 && len(code) != tt.length {
				t.Errorf("generateRandomCode(%d) = %q, length %d, want %d", 
					tt.length, code, len(code), tt.length)
			}
			
			// Verify code contains only valid characters
			if tt.length > 0 {
				for _, ch := range code {
					isValid := false
					for _, validCh := range alphabet {
						if byte(ch) == validCh {
							isValid = true
							break
						}
					}
					if !isValid {
						t.Errorf("generateRandomCode(%d) = %q, contains invalid character %c", 
							tt.length, code, ch)
					}
				}
			}
		})
	}
}