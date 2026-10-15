package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Server handles HTTP requests
type Server struct {
	store      *Store
	adminToken string
}

// NewServer creates a new HTTP server
func NewServer(store *Store, adminToken string) *Server {
	return &Server{
		store:      store,
		adminToken: adminToken,
	}
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/api/links" && r.Method == http.MethodPost:
		s.createLink(w, r)
	case path == "/api/links" && r.Method == http.MethodGet:
		s.listLinks(w, r)
	case strings.HasPrefix(path, "/api/links/") && r.Method == http.MethodDelete:
		s.deleteLink(w, r)
	case len(path) > 1 && path[0] == '/' && path[1:] != "" && path != "/api":
		s.redirectLink(w, r)
	default:
		http.NotFound(w, r)
	}
}

// adminAuthMiddleware checks for valid admin token
func (s *Server) adminAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, http.StatusUnauthorized, "missing or invalid authorization")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != s.adminToken {
			jsonError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		next(w, r)
	}
}

// createLink handles POST /api/links
func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	defer r.Body.Close()

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		if err == io.EOF {
			jsonError(w, http.StatusBadRequest, "empty request body")
		} else if strings.Contains(err.Error(), "unknown field") {
			jsonError(w, http.StatusBadRequest, "unknown field in request")
		} else {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
		}
		return
	}

	// Validate URL
	if req.URL == "" {
		jsonError(w, http.StatusBadRequest, "url is required")
		return
	}
	if len(req.URL) > 2048 {
		jsonError(w, http.StatusBadRequest, "url too long")
		return
	}
	parsedURL, err := url.Parse(req.URL)
	if err != nil || !parsedURL.IsAbs() || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		jsonError(w, http.StatusBadRequest, "invalid url")
		return
	}

	var code string
	if req.Alias != "" {
		// Validate alias
		aliasRegex := regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
		if !aliasRegex.MatchString(req.Alias) {
			jsonError(w, http.StatusBadRequest, "invalid alias format")
			return
		}
		if req.Alias == "api" {
			jsonError(w, http.StatusBadRequest, "alias 'api' is not allowed")
			return
		}
		code = req.Alias
	} else {
		// Generate random code
		code = generateRandomCode(7)
		for s.store.CodeExists(code) {
			code = generateRandomCode(7)
		}
	}

	// Check if code already exists
	if s.store.CodeExists(code) {
		jsonError(w, http.StatusConflict, "alias already taken")
		return
	}

	// Create the link
	link, err := s.store.CreateLink(code, req.URL, req.Alias)
	if err != nil {
		if err.Error() == "code already exists" {
			jsonError(w, http.StatusConflict, "alias already taken")
		} else {
			jsonError(w, http.StatusInternalServerError, "failed to create link")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

// redirectLink handles GET /{code}
func (s *Server) redirectLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/")
	
	// Increment visits first (atomic with save)
	if err := s.store.IncrementVisits(code); err != nil {
		if err.Error() == "link not found" {
			http.NotFound(w, r)
		} else {
			jsonError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	// Get the link
	link, err := s.store.GetLink(code)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

// listLinks handles GET /api/links (admin only)
func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	s.adminAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		links := s.store.GetAllLinks()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(links)
	})(w, r)
}

// deleteLink handles DELETE /api/links/{code} (admin only)
func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request) {
	s.adminAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/api/links/")
		if code == "" {
			jsonError(w, http.StatusBadRequest, "code is required")
			return
		}

		if err := s.store.DeleteLink(code); err != nil {
			if err.Error() == "link not found" {
				jsonError(w, http.StatusNotFound, "link not found")
			} else {
				jsonError(w, http.StatusInternalServerError, "failed to delete link")
			}
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})(w, r)
}