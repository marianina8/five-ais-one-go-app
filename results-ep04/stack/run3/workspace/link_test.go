package main

import (
	"testing"
)

func TestLinkValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com/path", false},
		{"missing scheme", "example.com", true},
		{"invalid scheme", "ftp://example.com", true},
		{"too long", "https://example.com/" + string(make([]byte, 2049)), true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestAliasValidation(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		wantErr bool
	}{
		{"valid short", "abc", false},
		{"valid with dash", "my-page", false},
		{"valid with underscore", "my_page", false},
		{"valid with numbers", "page123", false},
		{"too short", "ab", true},
		{"too long", string(make([]byte, 33)), true},
		{"invalid characters", "page@123", true},
		{"reserved", "api", true},
		{"empty", "", false}, // empty should be ok (will generate random)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAlias(tt.alias)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAlias(%q) error = %v, wantErr %v", tt.alias, err, tt.wantErr)
			}
		})
	}
}

func TestGenerateRandomCode(t *testing.T) {
	// Test that generated code is unique
	codes := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code := generateRandomCode()
		if len(code) != 7 {
			t.Errorf("generateRandomCode() length = %d, want 7", len(code))
		}
		for _, ch := range code {
			if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
				t.Errorf("generateRandomCode() contains invalid character: %c", ch)
			}
		}
		if codes[code] {
			t.Errorf("generateRandomCode() generated duplicate code: %s", code)
		}
		codes[code] = true
	}
}