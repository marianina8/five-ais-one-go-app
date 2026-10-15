package main

import (
	"fmt"
	"net/url"
	"regexp"
)

var (
	ErrInvalidURL    = fmt.Errorf("invalid URL")
	ErrInvalidAlias  = fmt.Errorf("invalid alias")
	ErrAliasTaken    = fmt.Errorf("alias already taken")
	ErrAliasReserved = fmt.Errorf("alias 'api' is reserved")
)

var aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

func ValidateURL(urlStr string) error {
	if len(urlStr) > 2048 {
		return ErrInvalidURL
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return ErrInvalidURL
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}

	if u.Host == "" {
		return ErrInvalidURL
	}

	return nil
}

func ValidateAlias(alias string) error {
	if alias == "" {
		return nil // empty means generate
	}

	if alias == "api" {
		return ErrAliasReserved
	}

	if !aliasRegex.MatchString(alias) {
		return ErrInvalidAlias
	}

	return nil
}