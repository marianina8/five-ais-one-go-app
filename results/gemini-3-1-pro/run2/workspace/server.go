package main

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Server struct {
	store      *Store
	adminToken string
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func checkAuth(req *http.Request, token string) bool {
	auth := req.Header.Get("Authorization")
	return auth == "Bearer "+token
}

func generateCode() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 7)
	for i := range b {
		var r [1]byte
		rand.Read(r[:])
		b[i] = chars[int(r[0])%len(chars)]
	}
	return string(b)
}

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if err.Error() == "http: request body too large" {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "malformed json or unknown fields")
		return
	}

	// Validate URL
	u, err := url.ParseRequestURI(req.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || utf8.RuneCountInString(req.URL) > 2048 {
		writeError(w, http.StatusBadRequest, "invalid url")
		return
	}

	// Validate Alias
	code := req.Alias
	isAlias := false
	if code != "" {
		if !aliasRe.MatchString(code) || code == "api" {
			writeError(w, http.StatusBadRequest, "invalid alias")
			return
		}
		isAlias = true
	}

	var link *Link
	for {
		if !isAlias {
			code = generateCode()
			if code == "api" {
				continue
			}
		}

		link, err = s.store.Create(code, req.URL)
		if err == ErrDuplicateCode {
			if isAlias {
				writeError(w, http.StatusConflict, "alias already taken")
				return
			}
			continue // try another random code
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		break
	}

	writeJSON(w, http.StatusCreated, link)
}

func (s *Server) handleFollowLink(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "api" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	link, found, err := s.store.Visit(code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if !checkAuth(r, s.adminToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	links := s.store.List()
	writeJSON(w, http.StatusOK, links)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if !checkAuth(r, s.adminToken) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	code := r.PathValue("code")
	found, err := s.store.Delete(code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// interceptor to wrap 405s in JSON
type errorInterceptor struct {
	http.ResponseWriter
	status int
}

func (w *errorInterceptor) WriteHeader(code int) {
	w.status = code
	if code == 405 || code == 404 {
		if w.Header().Get("Content-Type") != "application/json" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Content-Type-Options", "nosniff")
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *errorInterceptor) Write(b []byte) (int, error) {
	if (w.status == 405 || w.status == 404) && w.Header().Get("Content-Type") == "application/json" {
		msg := strings.TrimSpace(string(b))
		if msg == "" || msg == "Method Not Allowed" || msg == "404 page not found" {
			if w.status == 404 {
				msg = "not found"
			} else {
				msg = "method not allowed"
			}
		}
		res, _ := json.Marshal(map[string]string{"error": msg})
		w.ResponseWriter.Write(res)
		w.ResponseWriter.Write([]byte("\n"))
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

func wrapInterceptor(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		iw := &errorInterceptor{ResponseWriter: w}
		h.ServeHTTP(iw, r)
	})
}
