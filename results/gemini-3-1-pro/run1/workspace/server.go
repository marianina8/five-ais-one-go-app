package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
)

var aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

type Server struct {
	store      *Store
	adminToken string
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + s.adminToken
		if auth != expected {
			writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB limit

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "Request body too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "Malformed JSON or unknown fields")
		return
	}

	if len(req.URL) > 2048 {
		writeJSONError(w, http.StatusBadRequest, "URL too long")
		return
	}

	u, err := url.ParseRequestURI(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeJSONError(w, http.StatusBadRequest, "Invalid URL")
		return
	}

	if req.Alias != "" {
		if req.Alias == "api" {
			writeJSONError(w, http.StatusBadRequest, "Alias cannot be 'api'")
			return
		}
		if !aliasRegex.MatchString(req.Alias) {
			writeJSONError(w, http.StatusBadRequest, "Invalid alias")
			return
		}
	}

	link, err := s.store.Create(req.URL, req.Alias)
	if err != nil {
		if err.Error() == "conflict" {
			writeJSONError(w, http.StatusConflict, "Alias already taken")
			return
		}
		log.Printf("Store create error: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (s *Server) handleGetLink(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	link, err := s.store.GetAndVisit(code)
	if err != nil {
		if err.Error() == "not found" {
			writeJSONError(w, http.StatusNotFound, "Not Found")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "Internal Server Error")
		}
		return
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	links := s.store.List()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(links)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	err := s.store.Delete(code)
	if err != nil {
		if err.Error() == "not found" {
			writeJSONError(w, http.StatusNotFound, "Not Found")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "Internal Server Error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type rwWrapper struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *rwWrapper) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.wroteHeader = true
	if code == http.StatusMethodNotAllowed || code == http.StatusNotFound {
		rw.ResponseWriter.Header().Set("Content-Type", "application/json")
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *rwWrapper) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	if rw.status == http.StatusMethodNotAllowed {
		msg, _ := json.Marshal(map[string]string{"error": "Method Not Allowed"})
		rw.ResponseWriter.Write(msg)
		return len(b), nil
	}
	if rw.status == http.StatusNotFound {
		msg, _ := json.Marshal(map[string]string{"error": "Not Found"})
		rw.ResponseWriter.Write(msg)
		return len(b), nil
	}
	return rw.ResponseWriter.Write(b)
}

func wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cw := &rwWrapper{ResponseWriter: w}
		next.ServeHTTP(cw, r)
	})
}

func NewRouter(store *Store, adminToken string) http.Handler {
	s := &Server{store: store, adminToken: adminToken}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/links", s.handleCreate)
	mux.HandleFunc("GET /api/links", s.requireAuth(s.handleList))
	mux.HandleFunc("DELETE /api/links/{code}", s.requireAuth(s.handleDelete))
	mux.HandleFunc("GET /{code}", s.handleGetLink)

	return wrap(mux)
}
