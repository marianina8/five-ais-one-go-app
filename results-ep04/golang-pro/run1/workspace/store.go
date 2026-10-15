package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Alias     string    `json:"alias,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type Store struct {
	mu       sync.RWMutex
	links    map[string]*Link // code -> Link
	aliases  map[string]string // alias -> code mapping
	dataFile string
}

func NewStore(dataFile string) (*Store, error) {
	store := &Store{
		links:    make(map[string]*Link),
		aliases:  make(map[string]string),
		dataFile: dataFile,
	}

	if err := store.load(); err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}

	return store, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.dataFile)
	if err != nil {
		// If file doesn't exist, start with empty store
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var links []*Link
	if err := json.NewDecoder(file).Decode(&links); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	// Rebuild maps
	s.links = make(map[string]*Link)
	s.aliases = make(map[string]string)
	for _, link := range links {
		s.links[link.Code] = link
		if link.Alias != "" {
			s.aliases[link.Alias] = link.Code
		}
	}

	return nil
}

func (s *Store) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	// Create a temporary file first
	tmpFile := s.dataFile + ".tmp"
	file, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(links); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomically replace the original file
	if err := os.Rename(tmpFile, s.dataFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	return s.save()
}

func (s *Store) GetLink(code string) (*Link, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if it's an alias
	if actualCode, ok := s.aliases[code]; ok {
		code = actualCode
	}

	link, ok := s.links[code]
	if !ok {
		return nil, false
	}
	
	// Return a copy to avoid race conditions
	copy := *link
	return &copy, true
}

func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	link, ok := s.links[code]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("link not found")
	}
	link.Visits++
	s.mu.Unlock()

	return s.save()
}

func (s *Store) CreateLink(url, alias string) (*Link, error) {
	s.mu.Lock()

	// Generate a unique code
	code := generateRandomCode()
	for {
		if _, exists := s.links[code]; !exists && code != "api" {
			break
		}
		code = generateRandomCode()
	}

	// If alias is provided, validate uniqueness
	if alias != "" {
		if _, exists := s.aliases[alias]; exists {
			s.mu.Unlock()
			return nil, fmt.Errorf("alias already taken")
		}
		if alias == "api" {
			s.mu.Unlock()
			return nil, fmt.Errorf("alias 'api' is reserved")
		}
	} else {
		alias = ""
	}

	link := &Link{
		Code:      code,
		URL:       url,
		Alias:     alias,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link
	if alias != "" {
		s.aliases[alias] = code
	}

	s.mu.Unlock()

	if err := s.save(); err != nil {
		// Rollback - need to lock again
		s.mu.Lock()
		delete(s.links, code)
		if alias != "" {
			delete(s.aliases, alias)
		}
		s.mu.Unlock()
		return nil, err
	}

	// Return a copy
	copy := *link
	return &copy, nil
}

func (s *Store) GetAllLinks() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}
	return links
}

func (s *Store) DeleteLink(code string) error {
	s.mu.Lock()
	link, exists := s.links[code]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("link not found")
	}

	delete(s.links, code)
	if link.Alias != "" {
		delete(s.aliases, link.Alias)
	}
	s.mu.Unlock()

	return s.save()
}