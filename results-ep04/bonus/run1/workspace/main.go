package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxURLLength    = 2048
	maxBodySize     = 1 << 20 // 1 MiB
	randomCodeLen   = 7
	codeCharset     = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

var (
	aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	store, err := NewJSONStore(*dataFile)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, adminToken)
	log.Printf("Starting server on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, server))
}

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Alias     string    `json:"alias,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type Store interface {
	Create(url, alias string) (*Link, error)
	Get(code string) (*Link, error)
	IncrementVisits(code string) error
	List() ([]Link, error)
	Delete(code string) error
	Close() error
}

type JSONStore struct {
	mu       sync.RWMutex
	filePath string
	data     map[string]Link
}

func NewJSONStore(filePath string) (*JSONStore, error) {
	store := &JSONStore{
		filePath: filePath,
		data:     make(map[string]Link),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if os.IsNotExist(err) {
		// File doesn't exist yet, that's ok
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&s.data); err != nil {
		// If the file is empty or invalid JSON, start fresh
		if err == io.EOF {
			s.data = make(map[string]Link)
			return nil
		}
		return err
	}
	return nil
}

func (s *JSONStore) save() error {
	file, err := os.Create(s.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(s.data)
}

func (s *JSONStore) Create(urlStr, alias string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate URL
	if err := ValidateURL(urlStr); err != nil {
		return nil, err
	}

	// Validate alias if provided
	if alias != "" {
		if err := ValidateAlias(alias); err != nil {
			return nil, err
		}
		if _, exists := s.data[alias]; exists {
			return nil, fmt.Errorf("alias already taken")
		}
	}

	// Generate code or use alias
	code := alias
	if code == "" {
		// Generate random unique code
		for i := 0; i < 100; i++ { // Try up to 100 times
			code = generateRandomCode(randomCodeLen)
			if _, exists := s.data[code]; !exists {
				break
			}
			if i == 99 {
				return nil, fmt.Errorf("failed to generate unique code")
			}
		}
	}

	// Create link
	link := Link{
		Code:      code,
		URL:       urlStr,
		Alias:     alias,
		CreatedAt: time.Now(),
		Visits:    0,
	}

	s.data[code] = link
	if err := s.save(); err != nil {
		return nil, err
	}

	return &link, nil
}

func generateRandomCode(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = codeCharset[bytes[i]%byte(len(codeCharset))]
	}
	return string(result)
}

func ValidateURL(urlStr string) error {
	if len(urlStr) > maxURLLength {
		return fmt.Errorf("URL too long (max %d characters)", maxURLLength)
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}

	if parsed.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

func ValidateAlias(alias string) error {
	if alias == "api" {
		return fmt.Errorf("alias 'api' is reserved")
	}

	if !aliasRegex.MatchString(alias) {
		return fmt.Errorf("alias must match ^[A-Za-z0-9_-]{3,32}$")
	}

	return nil
}

func (s *JSONStore) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.data[code]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return &link, nil
}

func (s *JSONStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.data[code]
	if !exists {
		return fmt.Errorf("not found")
	}
	link.Visits++
	s.data[code] = link
	return s.save()
}

func (s *JSONStore) List() ([]Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.data))
	for _, link := range s.data {
		links = append(links, link)
	}

	// Sort by CreatedAt oldest first
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links, nil
}

func (s *JSONStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[code]; !exists {
		return fmt.Errorf("not found")
	}
	delete(s.data, code)
	return s.save()
}

func (s *JSONStore) Close() error {
	return nil
}

type Server struct {
	store      Store
	adminToken string
}

func NewServer(store Store, adminToken string) *Server {
	return &Server{
		store:      store,
		adminToken: adminToken,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/links":
		s.handleListLinks(w, r)
	case r.Method == "POST" && r.URL.Path == "/api/links":
		s.handleCreateLink(w, r)
	case r.Method == "DELETE" && len(r.URL.Path) > 13 && r.URL.Path[:12] == "/api/links/" && r.URL.Path[12:] != "":
		code := r.URL.Path[12:]
		s.handleDeleteLink(w, r, code)
	case r.Method == "GET" && len(r.URL.Path) > 1 && r.URL.Path[0] == '/' && !strings.HasPrefix(r.URL.Path, "/api/"):
		code := r.URL.Path[1:]
		s.handleRedirect(w, r, code)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

type createLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req createLinkRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		if err == io.EOF {
			writeError(w, http.StatusBadRequest, "missing request body")
		} else if strings.Contains(err.Error(), "unknown field") {
			writeError(w, http.StatusBadRequest, "unknown field in JSON")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON")
		}
		return
	}

	// Check for empty URL
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url field is required")
		return
	}

	link, err := s.store.Create(req.URL, req.Alias)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "URL scheme must be") ||
			strings.Contains(err.Error(), "invalid URL") ||
			strings.Contains(err.Error(), "URL must have a host") ||
			strings.Contains(err.Error(), "alias must match") ||
			strings.Contains(err.Error(), "alias 'api' is reserved") ||
			strings.Contains(err.Error(), "URL too long"):
			writeError(w, http.StatusBadRequest, err.Error())
		case strings.Contains(err.Error(), "alias already taken"):
			writeError(w, http.StatusConflict, "alias already taken")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, link)
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request, code string) {
	link, err := s.store.Get(code)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Increment visits before redirecting
	if err := s.store.IncrementVisits(code); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	links, err := s.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, links)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if !s.isAdminAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := s.store.Delete(code); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) isAdminAuthorized(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if len(auth) < 7 || auth[:7] != "Bearer " {
		return false
	}
	return auth[7:] == s.adminToken
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}