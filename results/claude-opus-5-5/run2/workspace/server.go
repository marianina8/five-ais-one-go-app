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

const maxBody = 1 << 20

// Server is the HTTP handler.
type Server struct {
	store *Store
	token string
}

func NewServer(store *Store, token string) *Server {
	return &Server{store: store, token: token}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return false
	}
	tok := h[len(p):]
	return subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) == 1
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
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			writeJSON(w, http.StatusOK, s.store.List())
		default:
			methodNotAllowed(w, "GET, HEAD, POST")
		}
	case strings.HasPrefix(path, "/api/links/"):
		code := strings.TrimPrefix(path, "/api/links/")
		if code == "" || strings.Contains(code, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, "DELETE")
			return
		}
		if !s.authorized(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err := s.store.Delete(code); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeError(w, http.StatusNotFound, "link not found")
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case path == "/api" || strings.HasPrefix(path, "/api/"):
		writeError(w, http.StatusNotFound, "not found")
	default:
		code := strings.TrimPrefix(path, "/")
		if code == "" || strings.Contains(code, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if _, ok := s.store.Get(code); !ok {
				writeError(w, http.StatusNotFound, "link not found")
				return
			}
			methodNotAllowed(w, "GET, HEAD")
			return
		}
		var l Link
		var err error
		if r.Method == http.MethodHead {
			var ok bool
			l, ok = s.store.Get(code)
			if !ok {
				err = ErrNotFound
			}
		} else {
			l, err = s.store.Visit(code)
		}
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeError(w, http.StatusNotFound, "link not found")
			} else {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
		w.Header().Set("Location", l.URL)
		w.WriteHeader(http.StatusFound)
	}
}

type createReq struct {
	URL   *string `json:"url"`
	Alias *string `json:"alias"`
}

func validURL(raw string) bool {
	if raw == "" || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	return u.Host != "" && u.Hostname() != ""
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req createReq
	err := dec.Decode(&req)
	if err == nil {
		// ensure nothing but whitespace follows
		var extra json.RawMessage
		if e := dec.Decode(&extra); e != io.EOF {
			if e == nil {
				err = errors.New("trailing data")
			} else {
				err = e
			}
		}
	}
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.URL == nil || !validURL(*req.URL) {
		writeError(w, http.StatusBadRequest, "url must be an absolute http or https URL with a host, at most 2048 characters")
		return
	}
	alias := ""
	if req.Alias != nil {
		alias = *req.Alias
		if !aliasRe.MatchString(alias) || alias == "api" {
			writeError(w, http.StatusBadRequest, "invalid alias")
			return
		}
	}
	l, err := s.store.Create(*req.URL, alias)
	if err != nil {
		if errors.Is(err, ErrExists) {
			writeError(w, http.StatusConflict, "alias already taken")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, l)
}
