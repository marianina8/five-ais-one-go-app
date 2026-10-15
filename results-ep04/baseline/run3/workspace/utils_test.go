package main

import (
	"testing"
)

func TestIsValidURL(t *testing.T) {
	tests := []struct {
		url    string
		expect bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"https://example.com/path", true},
		{"https://example.com/path?query=value", true},
		{"ftp://example.com", false}, // wrong scheme
		{"example.com", false},        // no scheme
		{"https://", false},           // no host
		{"", false},                   // empty
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := isValidURL(tt.url); got != tt.expect {
				t.Errorf("isValidURL(%q) = %v, want %v", tt.url, got, tt.expect)
			}
		})
	}
}

func TestIsValidAlias(t *testing.T) {
	tests := []struct {
		alias  string
		expect bool
	}{
		{"abc", true},           // min length
		{"abc-123_def", true},   // valid chars
		{"a", false},            // too short
		{"ab", false},           // too short
		{"", false},             // empty
		{"abc!123", false},      // invalid char
		{"abc/123", false},      // invalid char
		{"very-long-alias-that-is-way-too-long", false}, // too long
		{"api", true}, // technically valid pattern, but we'll reject it separately
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			if got := isValidAlias(tt.alias); got != tt.expect {
				t.Errorf("isValidAlias(%q) = %v, want %v", tt.alias, got, tt.expect)
			}
		})
	}
}

func TestGenerateRandomCode(t *testing.T) {
	code := generateRandomCode(7)
	if len(code) != 7 {
		t.Errorf("generateRandomCode(7) = length %d, want 7", len(code))
	}

	// Check all characters are valid
	for _, c := range code {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			t.Errorf("generateRandomCode contains invalid character %c", c)
		}
	}

	// Generate multiple codes to ensure randomness (very basic check)
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := generateRandomCode(7)
		if codes[code] {
			t.Errorf("duplicate random code generated: %s", code)
		}
		codes[code] = true
	}
}