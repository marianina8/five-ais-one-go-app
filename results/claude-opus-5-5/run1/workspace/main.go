package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type Server struct {
	store *Store
	token string
}

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

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

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case p == "/api/links":
		switch r.Method {
		case http.MethodPost:
			s.create(w, r)
		case http.MethodGet, http.MethodHead:
			if !s.authed(r) {
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			writeJSON(w, http.StatusOK, s.store.List())
		default:
			methodNotAllowed(w, "GET, HEAD, POST")
		}
	case strings.HasPrefix(p, "/api/links/"):
		code := strings.TrimPrefix(p, "/api/links/")
		if code == "" || strings.Contains(code, "/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, "DELETE")
			return
		}
		if !s.authed(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err := s.store.Delete(code); err != nil {
			if errors.Is(err, errNotFound) {
				writeErr(w, http.StatusNotFound, "link not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		code := strings.TrimPrefix(p, "/")
		if code == "" || strings.Contains(code, "/") || code == "api" {
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
				writeErr(w, http.StatusNotFound, "link not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.Header().Set("Location", target)
		w.WriteHeader(http.StatusFound)
	}
}

func (s *Server) authed(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	tok := h[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) == 1
}

func validURL(raw string) bool {
	if raw == "" || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return false
	}
	sc := strings.ToLower(u.Scheme)
	if sc != "http" && sc != "https" {
		return false
	}
	return u.Host != "" && u.Hostname() != ""
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req struct {
		URL   *string `json:"url"`
		Alias *string `json:"alias"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeErr(w, http.StatusBadRequest, "invalid JSON: trailing data")
		return
	}
	if req.URL == nil || !validURL(*req.URL) {
		writeErr(w, http.StatusBadRequest, "url must be an absolute http or https URL of at most 2048 characters")
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
		if errors.Is(err, errTaken) {
			writeErr(w, http.StatusConflict, "alias already taken")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "data.json", "data file")
	flag.Parse()
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: ADMIN_TOKEN must be set")
		os.Exit(1)
	}
	st, err := OpenStore(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: loading data:", err)
		os.Exit(1)
	}
	srv := &Server{store: st, token: token}
	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
