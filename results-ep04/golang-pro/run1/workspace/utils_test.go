package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateRandomCode(t *testing.T) {
	// Test that codes are generated correctly
	code := generateRandomCode()
	
	if len(code) != 7 {
		t.Errorf("Expected code length 7, got %d", len(code))
	}
	
	// Test that code only contains allowed characters
	allowedRegex := regexp.MustCompile(`^[A-Za-z0-9]{7}$`)
	if !allowedRegex.MatchString(code) {
		t.Errorf("Code contains invalid characters: %s", code)
	}
	
	// Test multiple codes are different (very low probability of collision)
	code2 := generateRandomCode()
	if code == code2 {
		t.Error("Expected different random codes")
	}
}

func TestIsValidURL(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected bool
	}{
		// Valid URLs
		{"Valid HTTPS", "https://example.com", true},
		{"Valid HTTP", "http://example.com", true},
		{"Valid with path", "https://example.com/path/to/page", true},
		{"Valid with query", "https://example.com?query=test", true},
		{"Valid with port", "https://example.com:8080", true},
		
		// Invalid URLs
		{"Invalid scheme", "ftp://example.com", false},
		{"No scheme", "example.com", false},
		{"Empty host", "http://", false},
		{"Missing scheme separator", "https//example.com", false},
		{"Too long", "https://example.com/" + strings.Repeat("a", 2048), false},
		{"Invalid format", "://example.com", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidURL(tc.url)
			if result != tc.expected {
				t.Errorf("isValidURL(%q) = %v, expected %v", tc.url, result, tc.expected)
			}
		})
	}
}

func TestIsValidAlias(t *testing.T) {
	testCases := []struct {
		name     string
		alias    string
		expected bool
	}{
		// Valid aliases
		{"Valid lowercase", "myalias", true},
		{"Valid uppercase", "MYALIAS", true},
		{"Valid with numbers", "alias123", true},
		{"Valid with underscore", "my_alias", true},
		{"Valid with hyphen", "my-alias", true},
		{"Valid mixed", "My-Alias_123", true},
		{"Minimum length", "abc", true},
		{"Maximum length", "abcdefghijklmnopqrstuvwxyz012345", true}, // 32 chars
		
		// Invalid aliases
		{"Empty", "", false},
		{"Too short", "ab", false},
		{"Too long", "abcdefghijklmnopqrstuvwxyz0123456", false}, // 33 chars
		{"Contains spaces", "my alias", false},
		{"Contains special chars", "alias@test", false},
		{"Reserved name", "api", false},
		{"Starts with number", "123alias", true}, // Should be allowed
		{"Only underscores", "___", true}, // Should be allowed
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidAlias(tc.alias)
			if result != tc.expected {
				t.Errorf("isValidAlias(%q) = %v, expected %v", tc.alias, result, tc.expected)
			}
		})
	}
}

func TestValidateCreateLinkRequest(t *testing.T) {
	testCases := []struct {
		name     string
		req      CreateLinkRequest
		hasError bool
	}{
		// Valid requests
		{
			name: "Valid URL with alias",
			req: CreateLinkRequest{
				URL:   "https://example.com",
				Alias: "my-alias",
			},
			hasError: false,
		},
		{
			name: "Valid URL without alias",
			req: CreateLinkRequest{
				URL:   "http://example.com",
				Alias: "",
			},
			hasError: false,
		},
		
		// Invalid requests
		{
			name: "Invalid URL scheme",
			req: CreateLinkRequest{
				URL:   "ftp://example.com",
				Alias: "test",
			},
			hasError: true,
		},
		{
			name: "Invalid alias format",
			req: CreateLinkRequest{
				URL:   "https://example.com",
				Alias: "ab", // too short
			},
			hasError: true,
		},
		{
			name: "Reserved alias",
			req: CreateLinkRequest{
				URL:   "https://example.com",
				Alias: "api",
			},
			hasError: true,
		},
		{
			name: "Multiple errors",
			req: CreateLinkRequest{
				URL:   "invalid-url",
				Alias: "@bad",
			},
			hasError: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors, ok := validateCreateLinkRequest(tc.req)
			if tc.hasError && ok {
				t.Errorf("Expected errors for request %+v, got ok", tc.req)
			}
			if !tc.hasError && !ok {
				t.Errorf("Expected no errors for request %+v, got errors: %v", tc.req, errors)
			}
		})
	}
}