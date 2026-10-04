package main

import (
	"net/url"
	"regexp"
	"unicode/utf8"
)

const (
	maxURLLength  = 2048
	reservedAlias = "api" // would shadow the /api/... routes
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// validURL reports whether raw is an absolute http or https URL with a host name, at most
// 2048 characters long.
// url.Parse also rejects control characters, so a URL can't smuggle a header into Location.
func validURL(raw string) bool {
	if raw == "" || utf8.RuneCountInString(raw) > maxURLLength {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

func validAlias(alias string) bool {
	return aliasPattern.MatchString(alias) && alias != reservedAlias
}
