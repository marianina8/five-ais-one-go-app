package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

func main() {
	// Parse command-line flags
	addr := flag.String("addr", ":8080", "server address")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	// Check ADMIN_TOKEN environment variable
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	// Load or initialize storage
	store, err := NewFileStore(*dataFile)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}
	defer store.Close()

	// Create server
	s := NewServer(store, adminToken)

	// Start server
	log.Printf("Starting server on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, s))
}

// Link represents a shortened URL
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

// Server holds the application state
type Server struct {
	store      *FileStore
	adminToken string
}

func NewServer(store *FileStore, adminToken string) *Server {
	return &Server{
		store:      store,
		adminToken: adminToken,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle redirects for short codes for GET and HEAD requests
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		!strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/" {
		s.handleRedirect(w, r)
		return
	}

	// API routes
	switch r.URL.Path {
	case "/api/links":
		switch r.Method {
		case http.MethodPost:
			s.handleCreateLink(w, r)
		case http.MethodGet:
			s.handleListLinks(w, r)
		default:
			s.methodNotAllowed(w, r)
		}
	default:
		// Check if it's a DELETE request to /api/links/{code}
		if strings.HasPrefix(r.URL.Path, "/api/links/") && r.Method == http.MethodDelete {
			s.handleDeleteLink(w, r)
			return
		}
		s.notFound(w, r)
	}
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	
	link, err := s.store.IncrementVisits(code)
	if err != nil {
		s.notFound(w, r)
		return
	}

	// For HEAD requests, just set the status and Location header
	// For GET requests, do actual redirect
	if r.Method == http.MethodHead {
		w.Header().Set("Location", link.URL)
		w.WriteHeader(http.StatusFound)
	} else {
		http.Redirect(w, r, link.URL, http.StatusFound)
	}
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	// Check content length first
	if r.ContentLength > 1<<20 { // 1 MiB
		s.payloadTooLarge(w)
		return
	}

	// Limit request body size to 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	// Read the entire body to enforce the limit
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		if isRequestBodyTooLarge(err) {
			s.payloadTooLarge(w)
			return
		}
		s.badRequest(w, "Failed to read request body")
		return
	}

	// Now decode from the bytes we read
	type request struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}

	var req request
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		if err == io.EOF {
			s.badRequest(w, "Empty request body")
		} else if err == io.ErrUnexpectedEOF {
			s.badRequest(w, "Malformed JSON")
		} else {
			s.badRequest(w, "Invalid JSON")
		}
		return
	}

	// Validate URL
	if !s.isValidURL(req.URL) {
		s.badRequest(w, "URL must be an absolute http or https URL with a host")
		return
	}

	// Validate alias if provided
	if req.Alias != "" {
		if !s.isValidAlias(req.Alias) {
			s.badRequest(w, "Alias must match ^[A-Za-z0-9_-]{3,32}$ and not be 'api'")
			return
		}
		if req.Alias == "api" {
			s.badRequest(w, "Alias 'api' is reserved")
			return
		}
	}

	// Generate link
	link, err := s.store.CreateLink(req.URL, req.Alias)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			s.conflict(w, "Alias already taken")
		} else {
			s.internalError(w, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func isRequestBodyTooLarge(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "request body too large") ||
	       strings.Contains(errStr, "http: request body too large") ||
	       strings.Contains(errStr, "http: requested body too large")
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		s.unauthorized(w)
		return
	}

	links := s.store.ListLinks()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		s.unauthorized(w)
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	if err := s.store.DeleteLink(code); err != nil {
		s.notFound(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	if len(urlStr) > 2048 {
		return false
	}
	return true
}

func (s *Server) isValidAlias(alias string) bool {
	if alias == "api" {
		return false
	}
	matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]{3,32}$`, alias)
	return matched
}

func (s *Server) isAdmin(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	return token == s.adminToken
}

// Error responses
func (s *Server) badRequest(w http.ResponseWriter, message string) {
	s.errorResponse(w, http.StatusBadRequest, message)
}

func (s *Server) unauthorized(w http.ResponseWriter) {
	s.errorResponse(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.errorResponse(w, http.StatusNotFound, "Not found")
}

func (s *Server) conflict(w http.ResponseWriter, message string) {
	s.errorResponse(w, http.StatusConflict, message)
}

func (s *Server) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	s.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	log.Printf("Internal error: %v", err)
	s.errorResponse(w, http.StatusInternalServerError, "Internal server error")
}

func (s *Server) payloadTooLarge(w http.ResponseWriter) {
	s.errorResponse(w, http.StatusRequestEntityTooLarge, "Request body too large")
}

func (s *Server) errorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}