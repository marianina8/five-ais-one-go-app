package main

import (
	"regexp"
	"testing"
)

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"http://example.com/path", true},
		{"http://example.com/path?query=value", true},
		{"http://example.com:8080", true},
		{"http://user:pass@example.com", true},
		{"ftp://example.com", false},
		{"example.com", false},
		{"http://", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			result := isValidURL(tc.url)
			if result != tc.expected {
				t.Errorf("isValidURL(%q) = %v, expected %v", tc.url, result, tc.expected)
			}
		})
	}

	// Test length limit
	longURL := "http://example.com/" + string(make([]byte, 2048))
	if isValidURL(longURL) {
		t.Error("expected false for URL longer than 2048 characters")
	}
}

func TestIsValidAlias(t *testing.T) {
	tests := []struct {
		alias    string
		expected bool
	}{
		{"abc", true},
		{"abc-123", true},
		{"abc_123", true},
		{"AbC123", true},
		{"a-b-c-1-2-3", true},
		{"api", false},
		{"API", false},
		{"ApI", false},
		{"ab", false}, // too short
		{"", false},
		{"abc-", false}, // ends with hyphen
		{"-abc", false}, // starts with hyphen
		{"abc_def", true},
		{"abc@def", false},
		{"abc.", false},
		{string(make([]byte, 33)), false}, // too long
		{"_abc", false}, // starts with underscore
		{"abc_", false}, // ends with underscore
	}

	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			result := isValidAlias(tc.alias)
			if result != tc.expected {
				t.Errorf("isValidAlias(%q) = %v, expected %v", tc.alias, result, tc.expected)
			}
		})
	}
}

func TestRandomCode(t *testing.T) {
	// Test length
	code := randomCode(7)
	if len(code) != 7 {
		t.Errorf("randomCode(7) returned code of length %d", len(code))
	}

	// Test characters are from allowed set
	re := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	if !re.MatchString(code) {
		t.Errorf("randomCode generated invalid characters: %s", code)
	}

	// Test different lengths
	for _, length := range []int{1, 3, 10, 20} {
		code := randomCode(length)
		if len(code) != length {
			t.Errorf("randomCode(%d) returned code of length %d", length, len(code))
		}
	}
}