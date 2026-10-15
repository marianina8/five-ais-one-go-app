package main

import (
	"encoding/json"
	"fmt"
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

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle root path redirects
	if r.URL.Path == "/" {
		s.handleRoot(w, r)
		return
	}

	// Handle API routes
	if strings.HasPrefix(r.URL.Path, "/api/links") {
		s.handleAPIRoutes(w, r)
		return
	}

	// Handle redirects for short codes
	s.handleRedirect(w, r)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "URL Shortener")
}

func (s *Server) handleAPIRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	w.Header().Set("Content-Type", "application/json")

	// Routes without code parameter
	if path == "/api/links" {
		switch r.Method {
		case http.MethodPost:
			s.createLink(w, r)
		case http.MethodGet:
			s.listLinks(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		}
		return
	}

	// Routes with code parameter
	if strings.HasPrefix(path, "/api/links/") {
		code := strings.TrimPrefix(path, "/api/links/")
		switch r.Method {
		case http.MethodDelete:
			s.deleteLink(w, r, code)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		}
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	url, err := s.store.IncrementVisits(code)
	if err != nil {
		if err == ErrNotFound {
			http.NotFound(w, r)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
		}
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) isAdminAuthorized(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	return token == s.adminToken
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.isAdminAuthorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}