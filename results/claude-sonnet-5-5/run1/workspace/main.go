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

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) bool {
	want := "Bearer " + s.token
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case p == "/api/links":
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
	case strings.HasPrefix(p, "/api/links/") && len(p) > len("/api/links/") && !strings.Contains(p[len("/api/links/"):], "/"):
		code := p[len("/api/links/"):]
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, "DELETE")
			return
		}
		if !s.admin(w, r) {
			return
		}
		switch err := s.store.Delete(code); {
		case errors.Is(err, ErrNotFound):
			writeErr(w, http.StatusNotFound, "not found")
		case err != nil:
			writeErr(w, http.StatusInternalServerError, "internal error")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	case len(p) > 1 && p[0] == '/' && !strings.Contains(p[1:], "/"):
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET, HEAD")
			return
		}
		target, err := s.store.Visit(p[1:], r.Method == http.MethodGet)
		switch {
		case errors.Is(err, ErrNotFound):
			writeErr(w, http.StatusNotFound, "not found")
		case err != nil:
			writeErr(w, http.StatusInternalServerError, "internal error")
		default:
			w.Header().Set("Location", target)
			w.WriteHeader(http.StatusFound)
		}
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

type createReq struct {
	URL   string `json:"url"`
	Alias string `json:"alias"`
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
	if len(req.URL) > 2048 || req.URL == "" {
		writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		writeErr(w, http.StatusBadRequest, "url must be an absolute http or https URL")
		return
	}
	if req.Alias != "" && (!aliasRe.MatchString(req.Alias) || req.Alias == "api") {
		writeErr(w, http.StatusBadRequest, "invalid alias")
		return
	}
	l, err := s.store.Create(req.URL, req.Alias)
	switch {
	case errors.Is(err, ErrExists):
		writeErr(w, http.StatusConflict, "alias already taken")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusCreated, l)
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
	log.Fatal(http.ListenAndServe(*addr, &Server{store: st, token: token}))
}
