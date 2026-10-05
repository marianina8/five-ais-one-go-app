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
	if strings.HasPrefix(h, p) && subtle.ConstantTimeCompare([]byte(h[len(p):]), []byte(s.token)) == 1 {
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
				writeJSON(w, http.StatusOK, s.store.List())
			}
		default:
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasPrefix(path, "/api/links/"):
		code := strings.TrimPrefix(path, "/api/links/")
		if code == "" || strings.Contains(code, "/") {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodDelete {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !s.admin(w, r) {
			return
		}
		if err := s.store.Delete(code); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
			} else {
				writeErr(w, http.StatusInternalServerError, "storage error")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case path == "/" || path == "/api" || strings.HasPrefix(path, "/api/") || strings.Contains(path[1:], "/"):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		code := path[1:]
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if r.Method == http.MethodHead {
			l, ok := s.store.Lookup(code)
			if !ok {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			w.Header().Set("Location", l.URL)
			w.WriteHeader(http.StatusFound)
			return
		}
		l, err := s.store.Visit(code)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
			} else {
				writeErr(w, http.StatusInternalServerError, "storage error")
			}
			return
		}
		w.Header().Set("Location", l.URL)
		w.WriteHeader(http.StatusFound)
	}
}

type createReq struct {
	URL   string `json:"url"`
	Alias string `json:"alias"`
}

func validURL(s string) bool {
	if s == "" || len(s) > 2048 {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || !u.IsAbs() {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Hostname() != ""
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req createReq
	err := dec.Decode(&req)
	if err == nil {
		if _, e2 := dec.Token(); e2 != io.EOF {
			err = errors.New("trailing data")
			if e2 != nil {
				err = e2
			}
		}
	}
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
		}
		return
	}
	if !validURL(req.URL) {
		writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	if req.Alias != "" && (!aliasRe.MatchString(req.Alias) || req.Alias == "api") {
		writeErr(w, http.StatusBadRequest, "invalid alias")
		return
	}
	l, err := s.store.Create(req.URL, req.Alias)
	if err != nil {
		if errors.Is(err, ErrTaken) {
			writeErr(w, http.StatusConflict, "alias already taken")
		} else {
			writeErr(w, http.StatusInternalServerError, "storage error")
		}
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
		fmt.Fprintln(os.Stderr, "error: cannot load data file:", err)
		os.Exit(1)
	}
	log.Fatal(http.ListenAndServe(*addr, &Server{store: st, token: token}))
}
