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
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

type Store struct {
	mu      sync.RWMutex
	links   map[string]*Link
	file    string
	nextID  int64
}

func NewStore(file string) (*Store, error) {
	s := &Store{
		links: make(map[string]*Link),
		file:  file,
	}

	// Load existing data if file exists
	if err := s.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// File doesn't exist, start with empty store
	}

	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.file)
	if err != nil {
		return err
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("failed to parse data file: %w", err)
	}

	s.links = make(map[string]*Link, len(links))
	for i := range links {
		link := &links[i]
		s.links[link.Code] = link
	}

	return nil
}

func (s *Store) save() error {
	// Caller must hold appropriate lock
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Write to a temporary file first, then rename (atomic write)
	tmpFile := s.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, s.file); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.save()
}

func (s *Store) CreateLink(url string, alias string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate URL
	if !isValidURL(url) {
		return nil, fmt.Errorf("invalid URL")
	}

	// Validate alias or generate code
	var code string
	if alias != "" {
		if !isValidAlias(alias) {
			return nil, fmt.Errorf("invalid alias")
		}
		if alias == "api" {
			return nil, fmt.Errorf("alias 'api' is not allowed")
		}
		code = alias
	} else {
		// Generate unique random code
		for {
			code = generateRandomCode(7)
			if _, exists := s.links[code]; !exists {
				break
			}
		}
	}

	// Check if code already exists
	if _, exists := s.links[code]; exists {
		return nil, fmt.Errorf("code already exists")
	}

	link := &Link{
		Code:      code,
		URL:       url,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link

	// Save to disk before returning
	if err := s.save(); err != nil {
		delete(s.links, code) // Rollback if save fails
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return link, nil
}

func (s *Store) GetLink(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, fmt.Errorf("link not found")
	}

	return link, nil
}

func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return fmt.Errorf("link not found")
	}

	link.Visits++

	// Save to disk before returning
	if err := s.save(); err != nil {
		link.Visits-- // Rollback if save fails
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *Store) GetAllLinks() []*Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	// Sort by CreatedAt (oldest first) - we'll handle this in the server
	return links
}

func (s *Store) DeleteLink(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("link not found")
	}

	delete(s.links, code)

	// Save to disk before returning
	if err := s.save(); err != nil {
		// Can't really rollback a delete easily
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}