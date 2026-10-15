package main

import (
	"math/rand/v2"
	"net/url"
	"strings"
)

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func isValidURL(rawURL string) bool {
	if len(rawURL) > 2048 {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	if u.Host == "" {
		return false
	}

	return true
}

func isValidAlias(alias string) bool {
	return aliasRegex.MatchString(alias)
}

func generateRandomCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}
	return string(b)
}

// Helper function to check if a path is an API path
func isAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/")
}