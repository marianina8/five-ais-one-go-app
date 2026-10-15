package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
)

const (
	codeLength     = 7
	allowedChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	maxURLLength   = 2048
	maxRequestBody = 1 * 1024 * 1024 // 1 MiB
)

var (
	aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
)

func generateRandomCode() string {
	var result strings.Builder
	result.Grow(codeLength)

	for i := 0; i < codeLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(allowedChars))))
		if err != nil {
			// Fallback to a simpler method if crypto/rand fails
			panic(fmt.Sprintf("failed to generate random number: %v", err))
		}
		result.WriteByte(allowedChars[n.Int64()])
	}

	return result.String()
}

func isValidURL(urlStr string) bool {
	if len(urlStr) > maxURLLength {
		return false
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	if parsed.Host == "" {
		return false
	}

	return true
}

func isValidAlias(alias string) bool {
	if alias == "api" {
		return false
	}
	return aliasRegex.MatchString(alias)
}

func validateCreateLinkRequest(req CreateLinkRequest) ([]string, bool) {
	var errors []string

	if !isValidURL(req.URL) {
		errors = append(errors, "url must be a valid http or https URL with a host, and at most 2048 characters")
	}

	if req.Alias != "" && !isValidAlias(req.Alias) {
		errors = append(errors, "alias must match ^[A-Za-z0-9_-]{3,32}$ and not be 'api'")
	}

	return errors, len(errors) == 0
}