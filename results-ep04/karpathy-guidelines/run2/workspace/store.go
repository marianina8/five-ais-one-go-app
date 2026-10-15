package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type JSONStore struct {
	filename string
	mu       sync.RWMutex
	links    map[string]Link
}

func NewJSONStore(filename string) (*JSONStore, error) {
	store := &JSONStore{
		filename: filename,
		links:    make(map[string]Link),
	}

	if err := store.load(); err != nil {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return store, nil
}

func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("invalid JSON data: %w", err)
	}

	s.links = make(map[string]Link)
	for _, link := range links {
		s.links[link.Code] = link
	}

	return nil
}

func (s *JSONStore) save() error {
	// This method is called from methods that already hold the lock
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	tempFile := s.filename + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempFile, s.filename)
}

var (
	aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	codeChars  = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
)

func generateRandomCode() string {
	b := make([]byte, 7)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		// Fallback to time-based generation if crypto/rand fails
		nano := time.Now().UnixNano()
		for i := range b {
			b[i] = codeChars[int(nano>>uint(i*8))%len(codeChars)]
		}
	}
	for i := range b {
		b[i] = codeChars[int(b[i])%len(codeChars)]
	}
	return string(b)
}

func (s *JSONStore) Create(urlStr, alias string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate URL
	if len(urlStr) > 2048 {
		return nil, errors.New("URL too long")
	}
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return nil, errors.New("URL must start with http:// or https://")
	}
	
	// Parse URL to ensure it's valid
	if _, err := url.ParseRequestURI(urlStr); err != nil {
		return nil, errors.New("invalid URL")
	}

	// Generate or validate code
	code := alias
	if code == "" {
		// Generate unique random code
		for i := 0; i < 100; i++ { // Try up to 100 times
			code = generateRandomCode()
			if _, exists := s.links[code]; !exists {
				break
			}
			code = "" // Reset if not unique
		}
		if code == "" {
			return nil, errors.New("failed to generate unique code")
		}
	} else {
		// Validate alias
		if code == "api" {
			return nil, errors.New("alias 'api' is reserved")
		}
		if !aliasRegex.MatchString(code) {
			return nil, errors.New("alias must match ^[A-Za-z0-9_-]{3,32}$")
		}
	}

	// Check if code already exists
	if _, exists := s.links[code]; exists {
		return nil, errors.New("alias already taken")
	}

	link := Link{
		Code:      code,
		URL:       urlStr,
		CreatedAt: time.Now().Format(time.RFC3339),
		Visits:    0,
	}

	s.links[code] = link

	if err := s.save(); err != nil {
		delete(s.links, code)
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return &link, nil
}

func (s *JSONStore) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, errors.New("not found")
	}
	return &link, nil
}

func (s *JSONStore) Visit(code string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return nil, errors.New("not found")
	}

	link.Visits++
	s.links[code] = link

	if err := s.save(); err != nil {
		// Rollback visit count
		link.Visits--
		s.links[code] = link
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return &link, nil
}

func (s *JSONStore) List() ([]Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	// Sort by creation time (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt < links[j].CreatedAt
	})

	return links, nil
}

func (s *JSONStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return errors.New("not found")
	}

	delete(s.links, code)

	return s.save()
}