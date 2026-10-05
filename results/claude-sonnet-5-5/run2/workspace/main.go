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

func (s *Server) methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
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

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/links":
		switch r.Method {
		case http.MethodPost:
			s.create(w, r)
		case http.MethodGet, http.MethodHead:
			if s.admin(w, r) {
				writeJSON(w, 200, s.store.List())
			}
		default:
			s.methodNotAllowed(w, "GET, HEAD, POST")
		}
	case strings.HasPrefix(path, "/api/links/") && !strings.Contains(path[len("/api/links/"):], "/"):
		code := path[len("/api/links/"):]
		if r.Method != http.MethodDelete {
			s.methodNotAllowed(w, "DELETE")
			return
		}
		if !s.admin(w, r) {
			return
		}
		if err := s.store.Delete(code); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, 404, "not found")
			} else {
				writeErr(w, 500, "internal error")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(path) > 1 && !strings.Contains(path[1:], "/") && path != "/api":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.methodNotAllowed(w, "GET, HEAD")
			return
		}
		l, err := s.store.Visit(path[1:])
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, 404, "not found")
			} else {
				writeErr(w, 500, "internal error")
			}
			return
		}
		w.Header().Set("Location", l.URL)
		w.WriteHeader(http.StatusFound)
	default:
		writeErr(w, 404, "not found")
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
			writeErr(w, 413, "request body too large")
			return
		}
		writeErr(w, 400, "invalid JSON")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, 413, "request body too large")
			return
		}
		writeErr(w, 400, "invalid JSON")
		return
	}
	if len(req.URL) > 2048 || req.URL == "" {
		writeErr(w, 400, "invalid url")
		return
	}
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		writeErr(w, 400, "url must be an absolute http or https URL")
		return
	}
	if req.Alias != "" && (!aliasRe.MatchString(req.Alias) || req.Alias == "api") {
		writeErr(w, 400, "invalid alias")
		return
	}
	l, err := s.store.Create(req.URL, req.Alias)
	if err != nil {
		if errors.Is(err, ErrExists) {
			writeErr(w, 409, "alias already taken")
		} else {
			writeErr(w, 500, "internal error")
		}
		return
	}
	writeJSON(w, 201, l)
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
