package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicateCode = errors.New("duplicate code")
	ErrInvalidAlias  = errors.New("invalid alias")
	ErrInvalidURL    = errors.New("invalid URL")
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Alias     string    `json:"alias,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type Store struct {
	mu        sync.RWMutex
	dataFile  string
	links     map[string]*Link
	byAlias   map[string]string // alias -> code
	codeGen   *codeGenerator
}

func NewStore(dataFile string) (*Store, error) {
	store := &Store{
		dataFile: dataFile,
		links:    make(map[string]*Link),
		byAlias:  make(map[string]string),
		codeGen:  newCodeGenerator(),
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.save()
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.dataFile)
	if os.IsNotExist(err) {
		// File doesn't exist yet, that's fine
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	var links []Link
	if err := json.NewDecoder(file).Decode(&links); err != nil {
		return err
	}

	// Rebuild maps
	for i := range links {
		link := links[i]
		s.links[link.Code] = &link
		if link.Alias != "" {
			s.byAlias[link.Alias] = link.Code
		}
		s.codeGen.markUsed(link.Code)
	}

	return nil
}

func (s *Store) save() error {
	s.mu.RLock()
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first
	tempFile := s.dataFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	// Rename temp file to actual file (atomic on most systems)
	return os.Rename(tempFile, s.dataFile)
}

func (s *Store) CreateLink(urlStr string, alias string) (*Link, error) {
	// Validate URL
	if err := validateURL(urlStr); err != nil {
		return nil, err
	}

	// Validate alias if provided
	if alias != "" {
		if err := validateAlias(alias); err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if alias already exists
	code := ""
	if alias != "" {
		if _, exists := s.byAlias[alias]; exists {
			return nil, ErrDuplicateCode
		}
		code = alias // Use alias as code
	} else {
		// Generate unique code
		for {
			code = s.codeGen.generate()
			if _, exists := s.links[code]; !exists {
				break
			}
		}
	}

	// Check if code already exists (shouldn't happen but be safe)
	if _, exists := s.links[code]; exists {
		return nil, ErrDuplicateCode
	}

	link := &Link{
		Code:      code,
		URL:       urlStr,
		Alias:     alias,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Store link
	s.links[code] = link
	if alias != "" {
		s.byAlias[alias] = code
	}
	s.codeGen.markUsed(code)

	// Persist to disk
	if err := s.save(); err != nil {
		// Rollback
		delete(s.links, code)
		if alias != "" {
			delete(s.byAlias, alias)
		}
		s.codeGen.unmarkUsed(code)
		return nil, err
	}

	return link, nil
}

func (s *Store) GetLink(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, ErrNotFound
	}
	return link, nil
}

func (s *Store) IncrementVisits(code string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return "", ErrNotFound
	}

	link.Visits++

	// Persist to disk
	if err := s.save(); err != nil {
		// Rollback visit count
		link.Visits--
		return "", err
	}

	return link.URL, nil
}

func (s *Store) ListLinks() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}
	// Sort by creation time (oldest first)
	// Since we're appending in map iteration order, we need to sort
	// We'll implement sorting in the handler for simplicity
	return links
}

func (s *Store) DeleteLink(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return ErrNotFound
	}

	// Remove from maps
	delete(s.links, code)
	if link.Alias != "" {
		delete(s.byAlias, link.Alias)
	}

	// Persist to disk
	if err := s.save(); err != nil {
		// Rollback
		s.links[code] = link
		if link.Alias != "" {
			s.byAlias[link.Alias] = code
		}
		return err
	}

	return nil
}

// Helper functions
func validateURL(urlStr string) error {
	if len(urlStr) > 2048 {
		return fmt.Errorf("%w: URL too long", ErrInvalidURL)
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be http or https", ErrInvalidURL)
	}

	if parsed.Host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}

	return nil
}

func validateAlias(alias string) error {
	if alias == "api" {
		return fmt.Errorf("%w: alias 'api' is reserved", ErrInvalidAlias)
	}

	// Match ^[A-Za-z0-9_-]{3,32}$
	if len(alias) < 3 || len(alias) > 32 {
		return fmt.Errorf("%w: alias must be 3-32 characters", ErrInvalidAlias)
	}

	matched, err := regexp.MatchString("^[A-Za-z0-9_-]+$", alias)
	if err != nil || !matched {
		return fmt.Errorf("%w: alias can only contain letters, numbers, underscores and hyphens", ErrInvalidAlias)
	}

	return nil
}