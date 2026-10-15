package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Server represents the HTTP server
type Server struct {
	store      *Store
	adminToken string
	mux        *http.ServeMux
}

// NewServer creates a new HTTP server
func NewServer(addr string, store *Store, adminToken string) *Server {
	if adminToken == "" {
		panic("admin token must not be empty")
	}

	s := &Server{
		store:      store,
		adminToken: adminToken,
		mux:        http.NewServeMux(),
	}

	// Set up routes
	s.mux.HandleFunc("POST /api/links", s.createLink)
	s.mux.HandleFunc("GET /api/links", s.listLinks)
	s.mux.HandleFunc("DELETE /api/links/", s.deleteLink)
	s.mux.HandleFunc("GET /", s.followLink)

	return s
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// createLink handles POST /api/links
func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias"`
	}

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&req)
	if err != nil {
		// Check if error is due to request body being too large
		if strings.Contains(err.Error(), "request body too large") {
			s.error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		s.error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate URL
	if req.URL == "" {
		s.error(w, "url is required", http.StatusBadRequest)
		return
	}
	if err := validateURL(req.URL); err != nil {
		s.error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate alias
	if err := validateAlias(req.Alias); err != nil {
		s.error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Determine code
	var code string
	if req.Alias != "" {
		code = req.Alias
		// Check if code already exists
		if _, err := s.store.Get(code); err == nil {
			s.error(w, fmt.Sprintf("alias %s already exists", code), http.StatusConflict)
			return
		}
	} else {
		code = s.store.findAvailableCode()
		// Ensure uniqueness (though findAvailableCode should guarantee this)
		for {
			if _, err := s.store.Get(code); err != nil {
				break // Code is unique
			}
			code = s.store.findAvailableCode()
		}
	}

	// Create link
	link := &Link{
		Code:      code,
		URL:       req.URL,
		CreatedAt: time.Now(),
		Visits:    0,
	}

	if err := s.store.Add(link); err != nil {
		// This shouldn't happen if we validated properly, but handle just in case
		s.error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

// followLink handles GET /{code}
func (s *Server) followLink(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" || code == "api" {
		s.notFound(w)
		return
	}

	link, err := s.store.Get(code)
	if err != nil {
		s.notFound(w)
		return
	}

	// Increment visit count
	if err := s.store.IncrementVisits(code); err != nil {
		// Log error but continue with redirect
		fmt.Printf("Failed to increment visits for %s: %v\n", code, err)
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

// listLinks handles GET /api/links (admin only)
func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		s.unauthorized(w)
		return
	}

	links := s.store.ListAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

// deleteLink handles DELETE /api/links/{code} (admin only)
func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	if code == "" {
		s.notFound(w)
		return
	}

	if !s.isAuthorized(r) {
		s.unauthorized(w)
		return
	}

	if err := s.store.Delete(code); err != nil {
		s.error(w, fmt.Sprintf("code %s not found", code), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// isAuthorized checks if the request has valid admin credentials
func (s *Server) isAuthorized(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	// Check for Bearer token
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}

	return parts[1] == s.adminToken
}

// error sends a JSON error response
func (s *Server) error(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// notFound sends a 404 error
func (s *Server) notFound(w http.ResponseWriter) {
	s.error(w, "not found", http.StatusNotFound)
}

// unauthorized sends a 401 error
func (s *Server) unauthorized(w http.ResponseWriter) {
	s.error(w, "unauthorized", http.StatusUnauthorized)
}