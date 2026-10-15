package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"
)

// Link represents a shortened URL
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Alias     string    `json:"alias,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

// Store manages links with thread-safe operations
type Store struct {
	mu       sync.RWMutex
	links    map[string]*Link
	dataFile string
	lastSave time.Time
}

// NewStore creates or loads a store from the given file
func NewStore(dataFile string) (*Store, error) {
	s := &Store{
		links:    make(map[string]*Link),
		dataFile: dataFile,
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return s, nil
}

// load reads links from the JSON file
func (s *Store) load() error {
	file, err := os.Open(s.dataFile)
	if err != nil {
		return err
	}
	defer file.Close()

	var links []Link
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&links); err != nil {
		if err == io.EOF {
			return nil // Empty file is OK
		}
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range links {
		link := links[i]
		s.links[link.Code] = &link
	}

	return nil
}

// save writes links to the JSON file - must be called with mu held
func (s *Store) save() error {
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	// Write to temp file first, then rename for atomic update
	tempFile := s.dataFile + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(links); err != nil {
		os.Remove(tempFile)
		return err
	}

	if err := file.Close(); err != nil {
		os.Remove(tempFile)
		return err
	}

	// Atomic rename
	if err := os.Rename(tempFile, s.dataFile); err != nil {
		os.Remove(tempFile)
		return err
	}

	s.lastSave = time.Now()
	return nil
}

// CreateLink adds a new link with the given code
func (s *Store) CreateLink(code, url, alias string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; exists {
		return nil, errors.New("code already exists")
	}

	link := &Link{
		Code:      code,
		URL:       url,
		Alias:     alias,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link

	if err := s.save(); err != nil {
		delete(s.links, code) // Rollback on save failure
		return nil, err
	}

	return link, nil
}

// GetLink retrieves a link by code
func (s *Store) GetLink(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, errors.New("link not found")
	}

	return link, nil
}

// IncrementVisits increments the visit count for a link
func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return errors.New("link not found")
	}

	link.Visits++

	if err := s.save(); err != nil {
		link.Visits-- // Rollback on save failure
		return err
	}

	return nil
}

// GetAllLinks returns all links, oldest first
func (s *Store) GetAllLinks() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	// Sort by creation time (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links
}

// DeleteLink removes a link by code
func (s *Store) DeleteLink(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return errors.New("link not found")
	}

	delete(s.links, code)

	if err := s.save(); err != nil {
		// Can't restore easily, but at least the in-memory state is consistent
		return err
	}

	return nil
}

// CodeExists checks if a code already exists
func (s *Store) CodeExists(code string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.links[code]
	return exists
}

// Close performs any cleanup
func (s *Store) Close() error {
	// No special cleanup needed for this implementation
	return nil
}