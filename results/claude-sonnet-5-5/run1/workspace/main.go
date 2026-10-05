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

func (s *Server) isAdmin(r *http.Request) bool {
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
		case http.MethodGet:
			if !s.isAdmin(r) {
				writeErr(w, 401, "unauthorized")
				return
			}
			writeJSON(w, 200, s.store.List())
		default:
			w.Header().Set("Allow", "GET, POST")
			writeErr(w, 405, "method not allowed")
		}
	case strings.HasPrefix(path, "/api/links/"):
		code := strings.TrimPrefix(path, "/api/links/")
		if code == "" || strings.Contains(code, "/") {
			writeErr(w, 404, "not found")
			return
		}
		if r.Method != http.MethodDelete {
			w.Header().Set("Allow", "DELETE")
			writeErr(w, 405, "method not allowed")
			return
		}
		if !s.isAdmin(r) {
			writeErr(w, 401, "unauthorized")
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
		w.WriteHeader(204)
	default:
		code := strings.TrimPrefix(path, "/")
		if code == "" || strings.Contains(code, "/") || code == "api" {
			writeErr(w, 404, "not found")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeErr(w, 405, "method not allowed")
			return
		}
		u, err := s.store.Visit(code)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, 404, "not found")
			} else {
				writeErr(w, 500, "internal error")
			}
			return
		}
		w.Header().Set("Location", u)
		w.WriteHeader(http.StatusFound)
	}
}

type createReq struct {
	URL   string `json:"url"`
	Alias string `json:"alias"`
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, 413, "request body too large")
		} else {
			writeErr(w, 400, "could not read body")
		}
		return
	}
	var req createReq
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if msg := validateURL(req.URL); msg != "" {
		writeErr(w, 400, msg)
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

func validateURL(s string) string {
	if s == "" {
		return "url is required"
	}
	if len(s) > 2048 {
		return "url too long"
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "url must be an absolute http or https URL"
	}
	return ""
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
