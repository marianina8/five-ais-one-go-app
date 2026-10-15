package main

import (
	"math/rand"
	"net/url"
	"regexp"
)

var (
	aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	alphabet   = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
)

func isValidURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Must have scheme http or https
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// Must have a host
	if u.Host == "" {
		return false
	}

	// URL must be absolute
	if !u.IsAbs() {
		return false
	}

	// Validate length (already checked in handler, but double-check)
	if len(rawURL) > 2048 {
		return false
	}

	// Ensure URL is properly formed
	return true
}

func isValidAlias(alias string) bool {
	return aliasRegex.MatchString(alias)
}

func generateRandomCode(length int) string {
	if length <= 0 {
		return ""
	}

	b := make([]byte, length)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}

// init seeds the random number generator for better randomness
func init() {
	rand.Seed(rand.Int63())
}