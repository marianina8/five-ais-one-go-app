package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"
)

var (
	ErrAliasTaken = errors.New("alias already taken")
	aliasRegex    = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

type Store struct {
	mu       sync.RWMutex
	links    map[string]*Link
	filePath string
	file     *os.File
}

func NewStore(filePath string) (*Store, error) {
	s := &Store{
		links:    make(map[string]*Link),
		filePath: filePath,
	}

	// Try to open existing file
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	s.file = file

	// Try to read existing data
	if err := s.load(); err != nil {
		file.Close()
		return nil, fmt.Errorf("load data: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if file is empty
	info, err := s.file.Stat()
	if err != nil {
		return err
	}

	if info.Size() == 0 {
		return nil
	}

	// Reset file pointer to beginning
	if _, err := s.file.Seek(0, 0); err != nil {
		return err
	}

	var links []Link
	if err := json.NewDecoder(s.file).Decode(&links); err != nil {
		return err
	}

	s.links = make(map[string]*Link)
	for i := range links {
		link := &links[i]
		s.links[link.Code] = link
	}

	return nil
}

func (s *Store) save() error {
	// Get copy of links while holding read lock
	s.mu.RLock()
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}
	s.mu.RUnlock()

	// Create temporary file
	tmpFile := s.filePath + ".tmp"
	file, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write data to temporary file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(links); err != nil {
		return err
	}

	// Ensure data is written to disk
	if err := file.Sync(); err != nil {
		return err
	}
	file.Close()

	// Replace original file with temporary file
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return err
	}

	// Reopen the file for future writes
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		s.file.Close()
	}

	newFile, err := os.OpenFile(s.filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	s.file = newFile

	return nil
}

func (s *Store) saveUnsafe() error {
	// This version doesn't acquire locks - caller must hold them
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	// Create temporary file
	tmpFile := s.filePath + ".tmp"
	file, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write data to temporary file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(links); err != nil {
		return err
	}

	// Ensure data is written to disk
	if err := file.Sync(); err != nil {
		return err
	}
	file.Close()

	// Replace original file with temporary file
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return err
	}

	// Reopen the file for future writes
	if s.file != nil {
		s.file.Close()
	}

	newFile, err := os.OpenFile(s.filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	s.file = newFile

	return nil
}

func (s *Store) CreateLink(rawURL, alias string) (*Link, error) {
	// Validate URL
	if !isValidURL(rawURL) {
		return nil, errors.New("invalid URL")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	code := alias
	if code == "" {
		// Generate unique random code
		for {
			code = generateRandomCode(7)
			if _, exists := s.links[code]; !exists {
				break
			}
		}
	} else {
		// Check if alias is valid
		if !isValidAlias(code) || code == "api" {
			return nil, errors.New("invalid alias")
		}

		// Check if alias already exists
		if _, exists := s.links[code]; exists {
			return nil, ErrAliasTaken
		}
	}

	link := &Link{
		Code:      code,
		URL:       rawURL,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link

	// Save to file before returning
	if err := s.saveUnsafe(); err != nil {
		// Rollback
		delete(s.links, code)
		return nil, err
	}

	return link, nil
}

func (s *Store) GetAndIncrement(code string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return nil, errors.New("link not found")
	}

	// Increment visits
	link.Visits++

	// Save to file before returning
	if err := s.saveUnsafe(); err != nil {
		// Rollback
		link.Visits--
		return nil, err
	}

	// Return a copy to avoid data races
	return &Link{
		Code:      link.Code,
		URL:       link.URL,
		CreatedAt: link.CreatedAt,
		Visits:    link.Visits,
	}, nil
}

func (s *Store) ListLinks() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	// Sort by creation time (oldest first)
	// Simple sort by timestamp
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].CreatedAt.Before(links[i].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}

	return links
}

func (s *Store) DeleteLink(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return errors.New("link not found")
	}

	delete(s.links, code)

	// Save to file before returning
	return s.saveUnsafe()
}