package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type jsonStore struct {
	mu       sync.RWMutex
	data     map[string]*Link
	file     string
	saveChan chan struct{}
}

func NewStore(filename string) (Store, error) {
	rand.Seed(time.Now().UnixNano()) // Initialize random seed
	
	store := &jsonStore{
		data:     make(map[string]*Link),
		file:     filename,
		saveChan: make(chan struct{}, 100), // Buffer to prevent blocking
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	go store.saveLoop()

	return store, nil
}

func (s *jsonStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.file)
	if os.IsNotExist(err) {
		return nil // Start with empty store
	}
	if err != nil {
		return fmt.Errorf("failed to open data file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read data file: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file
	}

	var links []*Link
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("failed to parse data file: %w", err)
	}

	s.data = make(map[string]*Link)
	for _, link := range links {
		s.data[link.Code] = link
	}

	return nil
}

func (s *jsonStore) save() error {
	s.mu.RLock()
	links := make([]*Link, 0, len(s.data))
	for _, link := range s.data {
		links = append(links, link)
	}
	s.mu.RUnlock()

	// Sort by creation time (oldest first)
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].CreatedAt.Before(links[i].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}

	data, err := json.Marshal(links)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Write to temp file first
	tempFile := s.file + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Rename atomically
	if err := os.Rename(tempFile, s.file); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (s *jsonStore) saveLoop() {
	for range s.saveChan {
		if err := s.save(); err != nil {
			log.Printf("failed to save store: %v", err)
		}
	}
}

func (s *jsonStore) triggerSave() {
	select {
	case s.saveChan <- struct{}{}:
	default:
		// Channel full, skip
	}
}

func (s *jsonStore) Create(urlStr, alias string) (*Link, error) {
	// Validate URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL must have a host")
	}
	if len(urlStr) > 2048 {
		return nil, fmt.Errorf("URL too long")
	}

	var code string
	if alias != "" {
		// Validate alias
		if alias == "api" {
			return nil, fmt.Errorf("alias cannot be 'api'")
		}
		match, _ := regexp.MatchString("^[A-Za-z0-9_-]{3,32}$", alias)
		if !match {
			return nil, fmt.Errorf("alias must match ^[A-Za-z0-9_-]{3,32}$")
		}
		code = alias
	} else {
		// Generate random code
		code = s.generateRandomCode()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if code exists
	if _, exists := s.data[code]; exists {
		return nil, fmt.Errorf("code already exists")
	}

	link := &Link{
		Code:      code,
		URL:       urlStr,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.data[code] = link
	s.triggerSave()

	return link, nil
}

func (s *jsonStore) generateRandomCode() string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	const length = 7

	for {
		var code strings.Builder
		for i := 0; i < length; i++ {
			code.WriteByte(chars[rand.Intn(len(chars))])
		}
		generated := code.String()
		
		// Check for uniqueness
		s.mu.RLock()
		_, exists := s.data[generated]
		s.mu.RUnlock()
		
		if !exists {
			return generated
		}
	}
}

func (s *jsonStore) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.data[code]
	if !exists {
		return nil, fmt.Errorf("link not found")
	}

	return link, nil
}

func (s *jsonStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.data[code]
	if !exists {
		return fmt.Errorf("link not found")
	}

	link.Visits++
	s.triggerSave()

	return nil
}

func (s *jsonStore) List() ([]*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*Link, 0, len(s.data))
	for _, link := range s.data {
		links = append(links, link)
	}

	// Sort by creation time (oldest first)
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].CreatedAt.Before(links[i].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}

	return links, nil
}

func (s *jsonStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[code]; !exists {
		return fmt.Errorf("link not found")
	}

	delete(s.data, code)
	s.triggerSave()

	return nil
}

func (s *jsonStore) Close() error {
	close(s.saveChan)
	// Wait for any pending saves
	time.Sleep(100 * time.Millisecond)
	return nil
}