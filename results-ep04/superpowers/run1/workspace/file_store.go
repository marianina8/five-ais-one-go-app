package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
)

// FileStore implements persistent storage with atomic writes
type FileStore struct {
	mu       sync.RWMutex
	links    map[string]Link
	filename string
}

// newFileStore creates a new file-backed store
func newFileStore(filename string) (*FileStore, error) {
	s := &FileStore{
		links:    make(map[string]Link),
		filename: filename,
	}

	// Try to load existing data
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return s, nil
}

// load reads the store from disk
func (s *FileStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filename)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		// Empty file
		return nil
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("failed to unmarshal store data: %w", err)
	}

	s.links = make(map[string]Link, len(links))
	for _, link := range links {
		s.links[link.Code] = link
	}

	return nil
}

// save writes the store to disk atomically
// Caller must hold at least a read lock
func (s *FileStore) save() error {
	// Convert map to sorted slice
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store data: %w", err)
	}

	// Write to temporary file first
	tmpFile := s.filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomically rename to target file
	if err := os.Rename(tmpFile, s.filename); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// Create adds a new link and saves to disk
func (fs *FileStore) Create(link Link) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.links[link.Code]; exists {
		return &StoreError{Code: "duplicate", Message: "alias already taken"}
	}

	fs.links[link.Code] = link
	
	// Save to disk
	if err := fs.save(); err != nil {
		// Rollback on save failure
		delete(fs.links, link.Code)
		return fmt.Errorf("failed to save store: %w", err)
	}

	return nil
}

// Get retrieves a link by code
func (fs *FileStore) Get(code string) (Link, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	link, exists := fs.links[code]
	if !exists {
		return Link{}, &StoreError{Code: "not_found", Message: "code not found"}
	}

	return link, nil
}

// IncrementVisits increments the visit count and saves to disk
func (fs *FileStore) IncrementVisits(code string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	link, exists := fs.links[code]
	if !exists {
		return &StoreError{Code: "not_found", Message: "code not found"}
	}

	link.Visits++
	fs.links[code] = link

	// Save to disk
	if err := fs.save(); err != nil {
		// Rollback on save failure
		link.Visits--
		fs.links[code] = link
		return fmt.Errorf("failed to save store: %w", err)
	}

	return nil
}

// Delete removes a link and saves to disk
func (fs *FileStore) Delete(code string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.links[code]; !exists {
		return &StoreError{Code: "not_found", Message: "code not found"}
	}

	// Save a copy for rollback
	oldLink := fs.links[code]
	delete(fs.links, code)

	// Save to disk
	if err := fs.save(); err != nil {
		// Rollback on save failure
		fs.links[code] = oldLink
		return fmt.Errorf("failed to save store: %w", err)
	}

	return nil
}

// List returns all links sorted by creation time
func (fs *FileStore) List() []Link {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	links := make([]Link, 0, len(fs.links))
	for _, link := range fs.links {
		links = append(links, link)
	}

	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links
}

// Close cleans up resources
func (fs *FileStore) Close() {
	// Nothing to do for file store
}