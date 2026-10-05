package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxBody = 1 << 20

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

func validAlias(alias string) bool {
	return alias != "api" && aliasPattern.MatchString(alias)
}

func validURL(raw string) bool {
	if utf8.RuneCountInString(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Hostname() != ""
}

type server struct {
	store *Store
	token [32]byte
}

func newHandler(store *Store, token string) http.Handler {
	return &server{store: store, token: sha256.Sum256([]byte("Bearer " + token))}
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, struct {
		Error string `json:"error"`
	}{message})
}

func methodError(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *server) authorized(w http.ResponseWriter, r *http.Request) bool {
	got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
	if subtle.ConstantTimeCompare(got[:], s.token[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func (s *server) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotFound):
		jsonError(w, http.StatusNotFound, errNotFound.Error())
	case errors.Is(err, errConflict):
		jsonError(w, http.StatusConflict, errConflict.Error())
	default:
		log.Printf("storage error: %v", err)
		jsonError(w, http.StatusInternalServerError, "storage error")
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/links":
		switch r.Method {
		case http.MethodPost:
			s.create(w, r)
		case http.MethodGet:
			if s.authorized(w, r) {
				jsonResponse(w, http.StatusOK, s.store.List())
			}
		default:
			methodError(w, "GET, POST")
		}
	case strings.HasPrefix(path, "/api/links/") && len(strings.TrimPrefix(path, "/api/links/")) > 0 && !strings.Contains(strings.TrimPrefix(path, "/api/links/"), "/"):
		if r.Method != http.MethodDelete {
			methodError(w, "DELETE")
			return
		}
		if !s.authorized(w, r) {
			return
		}
		if err := s.store.Delete(strings.TrimPrefix(path, "/api/links/")); err != nil {
			s.storeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(path) > 1 && strings.HasPrefix(path, "/") && !strings.Contains(path[1:], "/"):
		if r.Method != http.MethodGet {
			methodError(w, "GET")
			return
		}
		link, err := s.store.Visit(path[1:])
		if err != nil {
			s.storeError(w, err)
			return
		}
		w.Header().Set("Location", link.URL)
		w.WriteHeader(http.StatusFound)
	default:
		jsonError(w, http.StatusNotFound, "not found")
	}
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	// Read the whole bounded body first, so even malformed oversized JSON gets
	// 413 rather than a parser error caused by its prefix.
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			jsonError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			jsonError(w, http.StatusBadRequest, "invalid request body")
		}
		return
	}
	var request struct {
		URL   string          `json:"url"`
		Alias json.RawMessage `json:"alias"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validURL(request.URL) {
		jsonError(w, http.StatusBadRequest, "invalid URL")
		return
	}
	var alias string
	if request.Alias != nil {
		if err := json.Unmarshal(request.Alias, &alias); err != nil || !validAlias(alias) {
			jsonError(w, http.StatusBadRequest, "invalid alias")
			return
		}
	}
	link, err := s.store.Create(request.URL, alias)
	if err != nil {
		s.storeError(w, err)
		return
	}
	jsonResponse(w, http.StatusCreated, link)
}
