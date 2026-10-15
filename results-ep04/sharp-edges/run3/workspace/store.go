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

type JSONStore struct {
	mu       sync.RWMutex
	filename string
	links    map[string]*Link
}

func NewJSONStore(filename string) (*JSONStore, error) {
	store := &JSONStore{
		filename: filename,
		links:    make(map[string]*Link),
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filename)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's ok
			return nil
		}
		return err
	}

	// If file is empty, treat it as not existing
	if len(data) == 0 {
		return nil
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("failed to parse data file: %w", err)
	}

	s.links = make(map[string]*Link)
	for i := range links {
		link := links[i]
		s.links[link.Code] = &link
	}

	return nil
}

func (s *JSONStore) saveLocked() error {
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then rename for atomic updates
	tmpFile := s.filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, s.filename)
}

func (s *JSONStore) Create(url, code string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; exists {
		return nil, fmt.Errorf("code %s already exists", code)
	}

	link := &Link{
		Code:      code,
		URL:       url,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link

	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		return nil, err
	}

	return link, nil
}

func (s *JSONStore) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, fmt.Errorf("code %s not found", code)
	}

	return link, nil
}

func (s *JSONStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return fmt.Errorf("code %s not found", code)
	}

	link.Visits++

	return s.saveLocked()
}

func (s *JSONStore) List() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	return links
}

func (s *JSONStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("code %s not found", code)
	}

	delete(s.links, code)

	return s.saveLocked()
}

func (s *JSONStore) Close() error {
	return nil
}