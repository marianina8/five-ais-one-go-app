package main

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	codeChars    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	codeCharsLen = len(codeChars)
)

func randomCode(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("failed to generate random bytes: %v", err))
	}

	for i := range bytes {
		bytes[i] = codeChars[int(bytes[i])%codeCharsLen]
	}

	return string(bytes)
}

func isValidURL(rawURL string) bool {
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

	if len(rawURL) > 2048 {
		return false
	}

	return true
}

func isValidAlias(alias string) bool {
	if strings.ToLower(alias) == "api" {
		return false
	}

	re := regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	if !re.MatchString(alias) {
		return false
	}

	// Disallow starting or ending with dash or underscore
	if alias[0] == '-' || alias[0] == '_' || alias[len(alias)-1] == '-' || alias[len(alias)-1] == '_' {
		return false
	}

	return true
}