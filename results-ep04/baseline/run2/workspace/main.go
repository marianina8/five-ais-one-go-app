package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Link represents a shortened link
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

// Store manages links in memory and persists them to a JSON file
type Store struct {
	mu     sync.RWMutex
	links  map[string]*Link
	file   string
	saveMu sync.Mutex
}

// NewStore creates a new store that loads from and saves to the given file
func NewStore(file string) (*Store, error) {
	s := &Store{
		links: make(map[string]*Link),
		file:  file,
	}
	
	if err := s.load(); err != nil {
		return nil, err
	}
	
	return s, nil
}

// load reads links from the JSON file
func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	data, err := os.ReadFile(s.file)
	if err != nil {
		// File doesn't exist yet, that's OK
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	
	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return err
	}
	
	s.links = make(map[string]*Link)
	for i := range links {
		link := links[i]
		s.links[link.Code] = &link
	}
	
	return nil
}

// save writes all links to the JSON file
func (s *Store) save() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	
	// Collect links while holding the read lock
	s.mu.RLock()
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}
	s.mu.RUnlock()
	
	// Sort by CreatedAt for consistent ordering
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].CreatedAt.Before(links[i].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}
	
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}
	
	// Write to temporary file first
	tmpFile := s.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	
	// Rename atomically
	if err := os.Rename(tmpFile, s.file); err != nil {
		return err
	}
	
	return nil
}

// generateCode creates a random 7-character code
func generateCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	const length = 7
	
	code := make([]byte, length)
	for i := range code {
		code[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		// Use nanosleep to ensure different values if called in quick succession
		time.Sleep(1 * time.Nanosecond)
	}
	return string(code)
}

// Create creates a new link
func (s *Store) Create(originalURL, alias string) (*Link, error) {
	// Validate URL
	u, err := url.Parse(originalURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL must use http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL must have a host")
	}
	if len(originalURL) > 2048 {
		return nil, fmt.Errorf("URL too long")
	}
	
	// Determine code
	var code string
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if alias != "" {
		// Validate alias
		aliasRegex := regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
		if !aliasRegex.MatchString(alias) {
			return nil, fmt.Errorf("alias must match ^[A-Za-z0-9_-]{3,32}$")
		}
		if alias == "api" {
			return nil, fmt.Errorf("alias cannot be 'api'")
		}
		code = alias
	} else {
		// Generate unique random code
		for {
			code = generateCode()
			if _, exists := s.links[code]; !exists {
				break
			}
		}
	}
	
	// Check if code already exists
	if _, exists := s.links[code]; exists {
		return nil, fmt.Errorf("code already exists")
	}
	
	// Create link
	link := &Link{
		Code:      code,
		URL:       originalURL,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}
	
	s.links[code] = link
	
	// Release the lock before saving to avoid deadlock
	s.mu.Unlock()
	err = s.save()
	s.mu.Lock()
	
	if err != nil {
		delete(s.links, code)
		return nil, err
	}
	
	return link, nil
}

// Get retrieves a link by code and increments visits
func (s *Store) Get(code string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	link, exists := s.links[code]
	if !exists {
		return nil, fmt.Errorf("link not found")
	}
	
	// Increment visits
	link.Visits++
	
	// Release the lock before saving to avoid deadlock
	s.mu.Unlock()
	err := s.save()
	s.mu.Lock()
	
	if err != nil {
		// Rollback visit count
		link.Visits--
		return nil, err
	}
	
	return link, nil
}

// GetAll returns all links sorted by creation date
func (s *Store) GetAll() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}
	
	// Sort by CreatedAt (oldest first)
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].CreatedAt.Before(links[i].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}
	
	return links
}

// Delete removes a link by code
func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("link not found")
	}
	
	delete(s.links, code)
	
	// Release the lock before saving to avoid deadlock
	s.mu.Unlock()
	err := s.save()
	s.mu.Lock()
	
	if err != nil {
		// Can't really restore the link since we don't have it anymore
		return err
	}
	
	return nil
}

// Server handles HTTP requests
type Server struct {
	store       *Store
	adminToken  string
}

// NewServer creates a new server
func NewServer(store *Store, adminToken string) *Server {
	return &Server{
		store:      store,
		adminToken: adminToken,
	}
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes an error response
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// isAdminRequest checks if the request is properly authenticated as admin
func (s *Server) isAdminRequest(r *http.Request) bool {
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

// handleCreateLink handles POST /api/links
func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	
	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	
	link, err := s.store.Create(req.URL, req.Alias)
	if err != nil {
		switch err.Error() {
		case "code already exists":
			writeError(w, http.StatusConflict, err.Error())
		case "invalid URL", "URL must use http or https", "URL must have a host", "URL too long",
			"alias must match ^[A-Za-z0-9_-]{3,32}$", "alias cannot be 'api'":
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}
	
	writeJSON(w, http.StatusCreated, link)
}

// handleFollowLink handles GET /{code}
func (s *Server) handleFollowLink(w http.ResponseWriter, r *http.Request, code string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	link, err := s.store.Get(code)
	if err != nil {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	
	http.Redirect(w, r, link.URL, http.StatusFound)
}

// handleListLinks handles GET /api/links (admin)
func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	if !s.isAdminRequest(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	
	links := s.store.GetAll()
	writeJSON(w, http.StatusOK, links)
}

// handleDeleteLink handles DELETE /api/links/{code} (admin)
func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	if !s.isAdminRequest(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	
	if err := s.store.Delete(code); err != nil {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// ServeHTTP routes requests
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	
	switch {
	case path == "/api/links":
		if strings.HasSuffix(r.Method, "POST") {
			s.handleCreateLink(w, r)
		} else {
			s.handleListLinks(w, r)
		}
	case strings.HasPrefix(path, "/api/links/"):
		code := strings.TrimPrefix(path, "/api/links/")
		if code == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		s.handleDeleteLink(w, r, code)
	default:
		// Remove leading slash
		if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}
		if path == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		s.handleFollowLink(w, r, path)
	}
}

func main() {
	var addr, dataFile string
	flag.StringVar(&addr, "addr", ":8080", "address to listen on")
	flag.StringVar(&dataFile, "data", "data.json", "data file path")
	flag.Parse()
	
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}
	
	store, err := NewStore(dataFile)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	
	server := NewServer(store, adminToken)
	
	log.Printf("starting server on %s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatal(err)
	}
}