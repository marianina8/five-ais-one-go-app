package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"time"
)

// Link represents a shortened URL
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

var (
	aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	chars      = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
)

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}

// validateURL checks if a URL is valid according to spec
func validateURL(rawURL string) error {
	if len(rawURL) > 2048 {
		return fmt.Errorf("url too long, max 2048 characters")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must be http or https")
	}

	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// validateAlias checks if an alias is valid according to spec
func validateAlias(alias string) error {
	if alias == "" {
		return nil // empty means generate random
	}

	if alias == "api" {
		return fmt.Errorf("'api' is a reserved alias")
	}

	if !aliasRegex.MatchString(alias) {
		return fmt.Errorf("alias must match ^[A-Za-z0-9_-]{3,32}$")
	}

	return nil
}

// generateRandomCode generates a random 7-character code
func generateRandomCode() string {
	b := make([]byte, 7)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}