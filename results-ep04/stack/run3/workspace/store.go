package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
)

// Store manages links in memory and persists them to disk
type Store struct {
	path     string
	links    map[string]*Link // code -> Link
	mu       sync.RWMutex
}

// NewStore creates or loads a store from the given file path
func NewStore(path string) (*Store, error) {
	store := &Store{
		path:  path,
		links: make(map[string]*Link),
	}

	// Try to load existing data
	if err := store.load(); err != nil {
		// If file doesn't exist or is empty, that's ok
		if !os.IsNotExist(err) && err.Error() != "unexpected end of JSON input" {
			return nil, fmt.Errorf("failed to load store: %w", err)
		}
	}

	return store, nil
}

// load reads links from the JSON file
func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	var links []*Link
	if err := json.Unmarshal(data, &links); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, link := range links {
		s.links[link.Code] = link
	}

	return nil
}

// save writes links to the JSON file
func (s *Store) save() error {
	// Get copy of links for saving
	s.mu.RLock()
	links := make([]*Link, 0, len(s.links))
	for _, link := range s.links {
		// Create copy to avoid race conditions
		linkCopy := *link
		links = append(links, &linkCopy)
	}
	s.mu.RUnlock()

	// Sort by CreatedAt for deterministic output
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first, then rename
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path)
}

// Add adds a new link to the store
func (s *Store) Add(link *Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[link.Code]; exists {
		return fmt.Errorf("code %s already exists", link.Code)
	}

	s.links[link.Code] = link

	// Don't call save while lock is held - release lock first
	s.mu.Unlock()
	err := s.save()
	s.mu.Lock() // Re-acquire lock for defer statement

	return err
}

// Get retrieves a link by its code
func (s *Store) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, fmt.Errorf("code %s not found", code)
	}

	return link, nil
}

// IncrementVisits increments the visit count for a link
func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return fmt.Errorf("code %s not found", code)
	}

	link.Visits++

	// Don't call save while lock is held - release lock first
	s.mu.Unlock()
	err := s.save()
	s.mu.Lock() // Re-acquire lock for defer statement

	return err
}

// Delete removes a link from the store
func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("code %s not found", code)
	}

	delete(s.links, code)

	// Don't call save while lock is held - release lock first
	s.mu.Unlock()
	err := s.save()
	s.mu.Lock() // Re-acquire lock for defer statement

	return err
}

// ListAll returns all links, sorted by creation time (oldest first)
func (s *Store) ListAll() []*Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links
}

// findAvailableCode generates a unique random code
func (s *Store) findAvailableCode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for {
		code := generateRandomCode()
		if _, exists := s.links[code]; !exists {
			return code
		}
	}
}

// Close closes the store (no-op for now)
func (s *Store) Close() error {
	return nil
}