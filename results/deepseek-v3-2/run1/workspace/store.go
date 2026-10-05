package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"
)

// JSONStore implements Store interface using JSON file
type JSONStore struct {
	file     string
	mu       sync.RWMutex
	links    map[string]Link
	usedKeys map[string]bool
}

// NewJSONStore creates a new JSON store
func NewJSONStore(filename string) (*JSONStore, error) {
	store := &JSONStore{
		file:     filename,
		links:    make(map[string]Link),
		usedKeys: make(map[string]bool),
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

// load reads data from file
func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.file)
	if errors.Is(err, os.ErrNotExist) {
		// File doesn't exist yet, that's OK
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}

	for _, link := range links {
		s.links[link.Code] = link
		s.usedKeys[link.Code] = true
	}

	return nil
}

// save writes data to file
func (s *JSONStore) save() error {
	s.mu.RLock()
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	// Write to temporary file first
	tmpFile := s.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("writing temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, s.file); err != nil {
		return fmt.Errorf("renaming file: %w", err)
	}

	return nil
}

// saveLocked writes data to file (assumes lock is already held)
func (s *JSONStore) saveLocked() error {
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	// Write to temporary file first
	tmpFile := s.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("writing temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, s.file); err != nil {
		return fmt.Errorf("renaming file: %w", err)
	}

	return nil
}

// Create creates a new link
func (s *JSONStore) Create(urlStr, alias string) (*Link, error) {
	// Validate URL
	if err := validateURL(urlStr); err != nil {
		return nil, errors.New("invalid url")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate code
	var code string
	if alias != "" {
		if err := validateAlias(alias); err != nil {
			return nil, errors.New("invalid alias")
		}
		code = alias
	} else {
		// Generate unique random code
		for i := 0; i < 100; i++ { // Try up to 100 times
			code = generateRandomCode()
			if !s.usedKeys[code] {
				break
			}
			if i == 99 {
				return nil, errors.New("failed to generate unique code")
			}
		}
	}

	// Check if code exists (for aliases or random codes that collided)
	if s.usedKeys[code] {
		return nil, errors.New("alias taken")
	}

	// Create link
	link := Link{
		Code:      code,
		URL:       urlStr,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Store
	s.links[code] = link
	s.usedKeys[code] = true

	// Save to disk
	if err := s.saveLocked(); err != nil {
		// Rollback
		delete(s.links, code)
		delete(s.usedKeys, code)
		return nil, fmt.Errorf("save failed: %w", err)
	}

	return &link, nil
}

// Get retrieves a link by code
func (s *JSONStore) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, errors.New("not found")
	}

	return &link, nil
}

// IncrementVisits increments visit count for a link
func (s *JSONStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return errors.New("not found")
	}

	link.Visits++
	s.links[code] = link

	// Save to disk
	return s.saveLocked()
}

// List returns all links
func (s *JSONStore) List() ([]Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	// Sort by created_at (oldest first)
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].CreatedAt.Before(links[i].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}

	return links, nil
}

// Delete removes a link
func (s *JSONStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return errors.New("not found")
	}

	delete(s.links, code)
	delete(s.usedKeys, code)

	// Save to disk
	return s.saveLocked()
}

// validateURL validates URL according to spec
func validateURL(urlStr string) error {
	if len(urlStr) > 2048 {
		return errors.New("url too long")
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return err
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("invalid scheme")
	}

	if parsed.Host == "" {
		return errors.New("missing host")
	}

	return nil
}

// validateAlias validates alias according to spec
func validateAlias(alias string) error {
	if alias == "api" {
		return errors.New("reserved alias")
	}

	// Regex: ^[A-Za-z0-9_-]{3,32}$
	matched, err := regexp.MatchString(`^[A-Za-z0-9_-]{3,32}$`, alias)
	if err != nil || !matched {
		return errors.New("invalid alias format")
	}

	return nil
}

// generateRandomCode generates a random 7-character code
func generateRandomCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	const length = 7

	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}

	code := string(b)
	return code
}

// Initialize random seed
func init() {
	rand.Seed(time.Now().UnixNano())
}