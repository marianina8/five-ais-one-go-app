package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// FileStore handles persistence with thread-safe operations
type FileStore struct {
	file string
	mu   sync.RWMutex
	data storeData
}

type storeData struct {
	Links map[string]Link `json:"links"`
}

func NewFileStore(file string) (*FileStore, error) {
	store := &FileStore{
		file: file,
		data: storeData{
			Links: make(map[string]Link),
		},
	}

	// Try to load existing data
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return store, nil
}

func (s *FileStore) Close() error {
	return nil // Nothing to close
}

func (s *FileStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.file)
	if err != nil {
		return err
	}

	var loaded storeData
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("failed to parse store file: %w", err)
	}

	s.data = loaded
	return nil
}

func (s *FileStore) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store data: %w", err)
	}

	// Write to temporary file first for atomic update
	tmpFile := s.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Rename to final file (atomic on Unix-like systems)
	if err := os.Rename(tmpFile, s.file); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func (s *FileStore) CreateLink(url, alias string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	code := alias
	if code == "" {
		// Generate unique random code
		for i := 0; i < 100; i++ { // Try up to 100 times
			code = generateRandomCode(7)
			if _, exists := s.data.Links[code]; !exists {
				break
			}
		}
		if _, exists := s.data.Links[code]; exists {
			return nil, fmt.Errorf("failed to generate unique code")
		}
	}

	// Check if code already exists
	if _, exists := s.data.Links[code]; exists {
		return nil, fmt.Errorf("code %s already exists", code)
	}

	link := Link{
		Code:      code,
		URL:       url,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	// Store the link
	s.data.Links[code] = link

	// Save to disk
	if err := s.save(); err != nil {
		// Remove from memory if save fails
		delete(s.data.Links, code)
		return nil, fmt.Errorf("failed to save link: %w", err)
	}

	return &link, nil
}

func (s *FileStore) IncrementVisits(code string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.data.Links[code]
	if !exists {
		return nil, fmt.Errorf("link not found")
	}

	// Increment visits
	link.Visits++
	s.data.Links[code] = link

	// Save to disk
	if err := s.save(); err != nil {
		// Rollback the increment
		link.Visits--
		s.data.Links[code] = link
		return nil, fmt.Errorf("failed to save visit increment: %w", err)
	}

	return &link, nil
}

func (s *FileStore) ListLinks() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.data.Links))
	for _, link := range s.data.Links {
		links = append(links, link)
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

func (s *FileStore) DeleteLink(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data.Links[code]; !exists {
		return fmt.Errorf("link not found")
	}

	// Delete the link
	delete(s.data.Links, code)

	// Save to disk
	if err := s.save(); err != nil {
		// Re-add the link if save fails (we lost the original, would need to reload)
		return fmt.Errorf("failed to save after delete: %w", err)
	}

	return nil
}

func generateRandomCode(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	bytes := make([]byte, length)
	
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to time-based random if crypto/rand fails
		for i := range bytes {
			bytes[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		}
		return string(bytes)
	}

	result := make([]byte, length)
	for i := range bytes {
		result[i] = charset[bytes[i]%byte(len(charset))]
	}
	return string(result)
}