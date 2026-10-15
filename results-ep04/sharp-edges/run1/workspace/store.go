package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"
)

const (
	maxBodySize = 1 << 20 // 1 MiB
	codeLength  = 7
	codeChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Alias     string    `json:"alias,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

type Store struct {
	mu    sync.RWMutex
	links map[string]Link
	path  string
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		links: make(map[string]Link),
		path:  path,
	}

	if err := s.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// File doesn't exist, start with empty store
		if err := s.save(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.links = make(map[string]Link)
	for _, link := range links {
		s.links[link.Code] = link
	}

	return nil
}

func (s *Store) save() error {
	s.mu.RLock()
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	// Atomically rename to replace existing file
	return os.Rename(tmpPath, s.path)
}

func (s *Store) saveWithLock() error {
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	// Atomically rename to replace existing file
	return os.Rename(tmpPath, s.path)
}

func (s *Store) generateUniqueCode() string {
	for {
		code := makeRandomCode(codeLength)
		s.mu.RLock()
		_, exists := s.links[code]
		s.mu.RUnlock()
		if !exists {
			return code
		}
	}
}

func makeRandomCode(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	for i := range b {
		b[i] = codeChars[int(b[i])%len(codeChars)]
	}
	return string(b)
}

func (s *Store) Get(code string) (Link, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	link, exists := s.links[code]
	return link, exists
}

func (s *Store) Create(link Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if code already exists
	if _, exists := s.links[link.Code]; exists {
		return fmt.Errorf("code already exists")
	}

	s.links[link.Code] = link

	// Save to disk before returning - use saveWithLock since we already hold lock
	return s.saveWithLock()
}

func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return fmt.Errorf("code not found")
	}

	link.Visits++
	s.links[code] = link

	// Save to disk before returning
	return s.saveWithLock()
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("code not found")
	}

	delete(s.links, code)

	// Save to disk before returning
	return s.saveWithLock()
}

func (s *Store) List() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	// Sort by creation time, oldest first
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links
}

func (s *Store) authorizeAdmin(adminToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+adminToken {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Store) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate URL format first
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		jsonError(w, "invalid URL", http.StatusBadRequest)
		return
	}

	// Check URL has scheme and host
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		jsonError(w, "invalid URL", http.StatusBadRequest)
		return
	}
	if parsedURL.Host == "" {
		jsonError(w, "invalid URL", http.StatusBadRequest)
		return
	}

	// Check URL length
	if len(req.URL) > 2048 {
		jsonError(w, "URL too long", http.StatusBadRequest)
		return
	}

	// Validate alias if provided
	code := req.Alias
	if code != "" {
		if !isValidAlias(code) {
			jsonError(w, "invalid alias", http.StatusBadRequest)
			return
		}
		
		// Check if alias is taken
		s.mu.RLock()
		_, exists := s.links[code]
		s.mu.RUnlock()
		if exists {
			jsonError(w, "alias already taken", http.StatusConflict)
			return
		}
	} else {
		// Generate unique code
		code = s.generateUniqueCode()
	}

	// Create the link
	link := Link{
		Code:      code,
		URL:       req.URL,
		Alias:     req.Alias,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	if err := s.Create(link); err != nil {
		if err.Error() == "code already exists" {
			jsonError(w, "alias already taken", http.StatusConflict)
		} else {
			jsonError(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (s *Store) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	links := s.List()

	w.Header().Set("Content-Type", "application/json")
	if r.Method == "HEAD" {
		w.WriteHeader(http.StatusOK)
		return
	}
	json.NewEncoder(w).Encode(links)
}

func (s *Store) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		w.Header().Set("Allow", "DELETE")
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.PathValue("code")
	if code == "" {
		jsonError(w, "code required", http.StatusBadRequest)
		return
	}

	err := s.Delete(code)
	if err != nil {
		if err.Error() == "code not found" {
			jsonError(w, "not found", http.StatusNotFound)
		} else {
			jsonError(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Store) handleRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.PathValue("code")
	if code == "" {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	// Increment visits before redirecting
	if err := s.IncrementVisits(code); err != nil {
		if err.Error() == "code not found" {
			jsonError(w, "not found", http.StatusNotFound)
		} else {
			jsonError(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Get the URL to redirect to
	link, exists := s.Get(code)
	if !exists {
		// Should not happen since we just incremented visits
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
	
	// For HEAD requests, don't write response body
	if r.Method == "HEAD" {
		return
	}
	
	// For GET requests, write a simple redirect page
	fmt.Fprintf(w, "Redirecting to %s...", link.URL)
}