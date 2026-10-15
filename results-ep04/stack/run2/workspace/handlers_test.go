package main

import (
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want error
	}{
		{"valid https", "https://example.com", nil},
		{"valid http", "http://example.com", nil},
		{"no scheme", "example.com", ErrInvalidURL},
		{"ftp scheme", "ftp://example.com", ErrInvalidURL},
		{"too long", "https://" + strings.Repeat("a", 2049), ErrInvalidURL},
		{"empty", "", ErrInvalidURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateURL(tt.url)
			if (got == nil) != (tt.want == nil) {
				t.Errorf("ValidateURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestValidateAlias(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		want  error
	}{
		{"valid", "my-page-123", nil},
		{"too short", "ab", ErrInvalidAlias},
		{"too long", strings.Repeat("a", 33), ErrInvalidAlias},
		{"invalid chars", "my@page", ErrInvalidAlias},
		{"reserved api", "api", ErrInvalidAlias},
		{"empty", "", nil}, // empty is allowed (means generate)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAlias(tt.alias)
			if (got == nil) != (tt.want == nil) {
				t.Errorf("ValidateAlias(%q) = %v, want %v", tt.alias, got, tt.want)
			}
		})
	}
}