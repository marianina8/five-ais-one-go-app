package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// Server is the HTTP handler.
type Server struct {
	store *Store
	token string
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(h[len(p):]), []byte(s.token)) == 1
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/links":
		switch r.Method {
		case http.MethodPost:
			s.create(w, r)
		case http.MethodGet, http.MethodHead:
			if !s.authorized(r) {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			writeJSON(w, http.StatusOK, s.store.List())
		default:
			methodNotAllowed(w, "GET, HEAD, POST")
		}
	case strings.HasPrefix(path, "/api/links/"):
		code := strings.TrimPrefix(path, "/api/links/")
		if code == "" || strings.Contains(code, "/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, "DELETE")
			return
		}
		if !s.authorized(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err := s.store.Delete(code); err != nil {
			if errors.Is(err, errNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
			} else {
				writeErr(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case path == "/" || path == "/api" || strings.HasPrefix(path, "/api/"):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		code := path[1:]
		if strings.Contains(code, "/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET, HEAD")
			return
		}
		target, err := s.store.Visit(code)
		if err != nil {
			if errors.Is(err, errNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
			} else {
				writeErr(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusFound)
	}
}

func validURL(raw string) bool {
	if raw == "" || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return false
	}
	sch := strings.ToLower(u.Scheme)
	return (sch == "http" || sch == "https") && u.Host != ""
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		URL   *string `json:"url"`
		Alias *string `json:"alias"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&req)
	if err == nil {
		var extra json.RawMessage
		if e2 := dec.Decode(&extra); e2 != io.EOF {
			if e2 == nil {
				err = errors.New("trailing data")
			} else {
				err = e2
			}
		}
	}
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.URL == nil || !validURL(*req.URL) {
		writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	alias := ""
	if req.Alias != nil {
		alias = *req.Alias
		if !aliasRe.MatchString(alias) || alias == "api" {
			writeErr(w, http.StatusBadRequest, "invalid alias")
			return
		}
	}
	l, err := s.store.Create(*req.URL, alias)
	if err != nil {
		if errors.Is(err, errExists) {
			writeErr(w, http.StatusConflict, "alias already taken")
		} else {
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, l)
}
