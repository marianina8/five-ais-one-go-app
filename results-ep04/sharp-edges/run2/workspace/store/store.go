package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
	"shortener/models"
)

type Store struct {
	mu      sync.RWMutex
	file    string
	links   map[string]*models.Link
	aliases map[string]string // alias -> code mapping for quick lookup
}

func NewStore(file string) (*Store, error) {
	s := &Store{
		file:    file,
		links:   make(map[string]*models.Link),
		aliases: make(map[string]string),
	}
	
	if err := s.load(); err != nil {
		return nil, err
	}
	
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	data, err := os.ReadFile(s.file)
	if err != nil {
		// If file doesn't exist, start with empty store
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	
	// Handle empty file
	if len(data) == 0 {
		return nil
	}
	
	var links []*models.Link
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("failed to unmarshal data file: %w", err)
	}
	
	// Rebuild maps
	s.links = make(map[string]*models.Link)
	s.aliases = make(map[string]string)
	
	for _, link := range links {
		s.links[link.Code] = link
		// Track if code looks like an alias (alphanumeric with underscores/dashes)
		s.aliases[link.Code] = link.Code // For lookup purposes
	}
	
	return nil
}

func (s *Store) save() error {
	// Get copy of links while holding lock
	s.mu.RLock()
	links := make([]*models.Link, 0, len(s.links))
	for _, link := range s.links {
		// Create a copy to avoid race conditions
		linkCopy := *link
		links = append(links, &linkCopy)
	}
	s.mu.RUnlock()
	
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}
	
	// Atomic write: write to temp file, then rename
	tmpFile := s.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	
	if err := os.Rename(tmpFile, s.file); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	
	return nil
}

func (s *Store) CreateLink(code, url string) (*models.Link, error) {
	s.mu.Lock()
	
	// Check if code already exists
	if _, exists := s.links[code]; exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("code already exists")
	}
	
	link := &models.Link{
		Code:      code,
		URL:       url,
		CreatedAt: time.Now(),
		Visits:    0,
	}
	
	s.links[code] = link
	s.aliases[code] = code // For alias lookup
	
	// Need to unlock before save to avoid deadlock
	s.mu.Unlock()
	
	if err := s.save(); err != nil {
		// Rollback - need to lock again
		s.mu.Lock()
		delete(s.links, code)
		delete(s.aliases, code)
		s.mu.Unlock()
		return nil, fmt.Errorf("failed to save: %w", err)
	}
	
	return link, nil
}

func (s *Store) GetLink(code string) (*models.Link, error) {
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
	
	link, exists := s.links[code]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("link not found")
	}
	
	link.Visits++
	
	// Need to unlock before save to avoid deadlock
	s.mu.Unlock()
	
	if err := s.save(); err != nil {
		// Rollback - need to lock again
		s.mu.Lock()
		link.Visits--
		s.mu.Unlock()
		return fmt.Errorf("failed to save: %w", err)
	}
	
	return nil
}

func (s *Store) ListLinks() []*models.Link {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	links := make([]*models.Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}
	
	// Sort by CreatedAt (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})
	
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
	delete(s.aliases, code)
	
	// Need to unlock before save to avoid deadlock
	s.mu.Unlock()
	
	if err := s.save(); err != nil {
		// Rollback - need to lock again
		s.mu.Lock()
		s.links[code] = link
		s.aliases[code] = code
		s.mu.Unlock()
		return fmt.Errorf("failed to save: %w", err)
	}
	
	return nil
}

func (s *Store) CodeExists(code string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	_, exists := s.links[code]
	return exists
}