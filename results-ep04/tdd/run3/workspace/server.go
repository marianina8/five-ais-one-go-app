package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
)

type Server struct {
	store      *Store
	adminToken string
}

func NewServer(store *Store, adminToken string) *Server {
	return &Server{
		store:      store,
		adminToken: adminToken,
	}
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/", s.handleRoot)
	http.HandleFunc("/api/links", s.handleLinksAPI)
	http.HandleFunc("/api/links/", s.handleLinkAPI)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		s.sendError(w, http.StatusNotFound, "not found")
		return
	}

	// Increment visits before redirecting
	if err := s.store.IncrementVisits(path); err != nil {
		var notFoundErr *NotFoundError
		if errors.As(err, &notFoundErr) {
			s.sendError(w, http.StatusNotFound, "not found")
		} else {
			s.sendError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	link, err := s.store.Get(path)
	if err != nil {
		s.sendError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) handleLinksAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCreateLink(w, r)
	case http.MethodGet:
		s.handleListLinks(w, r)
	default:
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleLinkAPI(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	if code == "" {
		s.sendError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		s.handleDeleteLink(w, r, code)
	default:
		s.sendError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type CreateLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		s.sendError(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req CreateLinkRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		s.sendError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if !isValidURL(req.URL) {
		s.sendError(w, http.StatusBadRequest, "invalid URL")
		return
	}

	if req.Alias != "" && !isValidAlias(req.Alias) {
		s.sendError(w, http.StatusBadRequest, "invalid alias")
		return
	}

	link, err := s.store.Add(req.URL, req.Alias)
	if err != nil {
		var conflictErr *ConflictError
		if errors.As(err, &conflictErr) {
			s.sendError(w, http.StatusConflict, "alias already taken")
		} else {
			s.sendError(w, http.StatusInternalServerError, "failed to create link")
		}
		return
	}

	s.sendJSON(w, http.StatusCreated, link)
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		s.sendError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	links := s.store.List()
	s.sendJSON(w, http.StatusOK, links)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if !s.isAdmin(r) {
		s.sendError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := s.store.Delete(code); err != nil {
		var notFoundErr *NotFoundError
		if errors.As(err, &notFoundErr) {
			s.sendError(w, http.StatusNotFound, "not found")
		} else {
			s.sendError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) isAdmin(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	return token == s.adminToken
}

func (s *Server) sendError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *Server) sendJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode JSON: %v", err)
	}
}