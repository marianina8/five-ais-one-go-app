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

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

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

func (s *Server) admin(w http.ResponseWriter, r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) &&
		subtle.ConstantTimeCompare([]byte(h[len(p):]), []byte(s.token)) == 1 {
		return true
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeErr(w, http.StatusUnauthorized, "unauthorized")
	return false
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/links":
		switch r.Method {
		case http.MethodPost:
			s.create(w, r)
		case http.MethodGet, http.MethodHead:
			if s.admin(w, r) {
				writeJSON(w, http.StatusOK, s.store.List())
			}
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
		if !s.admin(w, r) {
			return
		}
		switch err := s.store.Delete(code); {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, ErrNotFound):
			writeErr(w, http.StatusNotFound, "not found")
		default:
			log.Printf("delete: %v", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
	default:
		code := strings.TrimPrefix(path, "/")
		if code == "" || code == "api" || strings.Contains(code, "/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET, HEAD")
			return
		}
		u, err := s.store.Visit(code)
		switch {
		case err == nil:
			w.Header().Set("Location", u)
			w.WriteHeader(http.StatusFound)
		case errors.Is(err, ErrNotFound):
			writeErr(w, http.StatusNotFound, "not found")
		default:
			log.Printf("visit: %v", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
		}
	}
}

type createReq struct {
	URL   *string `json:"url"`
	Alias *string `json:"alias"`
}

func validURL(s string) bool {
	if s == "" || len(s) > 2048 {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || !u.IsAbs() || u.Host == "" || u.Hostname() == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req createReq
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL == nil || !validURL(*req.URL) {
		writeErr(w, http.StatusBadRequest, "url must be an absolute http or https URL (max 2048 chars)")
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
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, l)
	case errors.Is(err, ErrExists):
		writeErr(w, http.StatusConflict, "alias already taken")
	default:
		log.Printf("create: %v", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
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
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	srv := &http.Server{Addr: *addr, Handler: &Server{store: st, token: token}}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
