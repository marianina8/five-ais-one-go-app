package main

import (
	"encoding/json"
	"net/http"
	"sort"
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
	// Handle URL redirects
	if r.Method == http.MethodGet && r.URL.Path != "/" && r.URL.Path != "/api/links" && !strings.HasPrefix(r.URL.Path, "/api/links/") {
		code := strings.TrimPrefix(r.URL.Path, "/")
		s.handleRedirect(w, r, code)
		return
	}

	switch r.URL.Path {
	case "/api/links":
		s.handleAPILinks(w, r)
	case "/":
		if r.Method == http.MethodGet {
			s.handleRoot(w, r)
		} else {
			s.sendMethodNotAllowed(w, r)
		}
	default:
		if strings.HasPrefix(r.URL.Path, "/api/links/") {
			code := strings.TrimPrefix(r.URL.Path, "/api/links/")
			s.handleAPILink(w, r, code)
		} else {
			s.sendNotFound(w, r)
		}
	}
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("URL Shortener Service\n"))
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request, code string) {
	link, err := s.store.GetLink(code)
	if err != nil {
		s.sendNotFound(w, r)
		return
	}

	// Increment visits
	if err := s.store.IncrementVisits(code); err != nil {
		s.sendInternalError(w, r, err)
		return
	}

	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
}

func (s *Server) handleAPILinks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCreateLink(w, r)
	case http.MethodGet:
		s.handleListLinks(w, r)
	default:
		s.sendMethodNotAllowed(w, r)
	}
}

func (s *Server) handleAPILink(w http.ResponseWriter, r *http.Request, code string) {
	switch r.Method {
	case http.MethodDelete:
		s.handleDeleteLink(w, r, code)
	default:
		s.sendMethodNotAllowed(w, r)
	}
}

type CreateLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate URL
	if req.URL == "" {
		s.sendBadRequest(w, r, "Missing URL")
		return
	}

	if len(req.URL) > 2048 {
		s.sendBadRequest(w, r, "URL too long")
		return
	}

	if !isValidURL(req.URL) {
		s.sendBadRequest(w, r, "Invalid URL format")
		return
	}

	link, err := s.store.CreateLink(req.URL, req.Alias)
	if err != nil {
		switch err.Error() {
		case "code already exists":
			s.sendConflict(w, r, "Alias already taken")
		case "invalid URL":
			s.sendBadRequest(w, r, "Invalid URL format")
		case "invalid alias":
			s.sendBadRequest(w, r, "Invalid alias format")
		case "alias 'api' is not allowed":
			s.sendBadRequest(w, r, "Alias 'api' is not allowed")
		default:
			s.sendInternalError(w, r, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	// Check admin token
	if !s.checkAdminAuth(r) {
		s.sendUnauthorized(w, r)
		return
	}

	links := s.store.GetAllLinks()
	
	// Sort by CreatedAt (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request, code string) {
	// Check admin token
	if !s.checkAdminAuth(r) {
		s.sendUnauthorized(w, r)
		return
	}

	if err := s.store.DeleteLink(code); err != nil {
		if err.Error() == "link not found" {
			s.sendNotFound(w, r)
		} else {
			s.sendInternalError(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) checkAdminAuth(r *http.Request) bool {
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

func (s *Server) sendJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) sendError(w http.ResponseWriter, status int, message string) {
	s.sendJSON(w, status, map[string]string{"error": message})
}

func (s *Server) sendBadRequest(w http.ResponseWriter, r *http.Request, message string) {
	s.sendError(w, http.StatusBadRequest, message)
}

func (s *Server) sendUnauthorized(w http.ResponseWriter, r *http.Request) {
	s.sendError(w, http.StatusUnauthorized, "Unauthorized")
}

func (s *Server) sendNotFound(w http.ResponseWriter, r *http.Request) {
	s.sendError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) sendConflict(w http.ResponseWriter, r *http.Request, message string) {
	s.sendError(w, http.StatusConflict, message)
}

func (s *Server) sendMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	s.sendError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
}

func (s *Server) sendInternalError(w http.ResponseWriter, r *http.Request, err error) {
	s.sendError(w, http.StatusInternalServerError, "Internal Server Error")
}