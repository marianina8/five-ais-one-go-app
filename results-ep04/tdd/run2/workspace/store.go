package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"
)

const (
	codeLength = 7
	codeChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	maxURLLen  = 2048
)

var aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

type jsonStore struct {
	filePath string
	mu       sync.RWMutex
	links    map[string]Link
}

type jsonStoreData struct {
	Links []Link `json:"links"`
}

func NewJSONStore(filePath string) (*jsonStore, error) {
	store := &jsonStore{
		filePath: filePath,
		links:    make(map[string]Link),
	}

	if err := store.load(); err != nil {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return store, nil
}

func (s *jsonStore) Create(urlStr, alias string) (*Link, error) {
	// Validate URL
	if err := validateURL(urlStr); err != nil {
		return nil, ErrInvalidURL
	}

	var code string
	var err error

	if alias != "" {
		if err := validateAlias(alias); err != nil {
			return nil, ErrInvalidAlias
		}
		code = alias
	} else {
		code, err = generateUniqueCode(s.links)
		if err != nil {
			return nil, fmt.Errorf("failed to generate unique code: %w", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if code already exists
	if _, exists := s.links[code]; exists {
		return nil, ErrAliasTaken
	}

	link := Link{
		Code:      code,
		URL:       urlStr,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link

	// Persist to disk before returning
	if err := s.save(); err != nil {
		// Rollback in memory
		delete(s.links, code)
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return &link, nil
}

func (s *jsonStore) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, ErrNotFound
	}

	return &link, nil
}

func (s *jsonStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return ErrNotFound
	}

	link.Visits++
	s.links[code] = link

	// Persist to disk before returning
	if err := s.save(); err != nil {
		// Rollback in memory
		link.Visits--
		s.links[code] = link
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *jsonStore) List() ([]Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	// Sort by creation time (oldest first)
	// Simple bubble sort for small datasets
	for i := 0; i < len(links)-1; i++ {
		for j := i + 1; j < len(links); j++ {
			if links[i].CreatedAt.After(links[j].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}

	return links, nil
}

func (s *jsonStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.links[code]
	if !exists {
		return ErrNotFound
	}

	delete(s.links, code)

	// Persist to disk before returning
	if err := s.save(); err != nil {
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *jsonStore) Close() error {
	return nil
}

func (s *jsonStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty store
			return nil
		}
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var data jsonStoreData
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil && err != io.EOF {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	s.links = make(map[string]Link)
	for _, link := range data.Links {
		s.links[link.Code] = link
	}

	return nil
}

func (s *jsonStore) save() error {
	// Write to temporary file first, then rename (atomic update)
	tempPath := s.filePath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		file.Close()
		// If we're still here and the file exists, clean it up
		if _, err := os.Stat(tempPath); err == nil {
			os.Remove(tempPath)
		}
	}()

	data := jsonStoreData{
		Links: make([]Link, 0, len(s.links)),
	}
	for _, link := range s.links {
		data.Links = append(data.Links, link)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, s.filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func generateUniqueCode(existingLinks map[string]Link) (string, error) {
	const maxAttempts = 1000
	for i := 0; i < maxAttempts; i++ {
		code, err := generateRandomCode()
		if err != nil {
			return "", fmt.Errorf("failed to generate random code: %w", err)
		}

		// Check if code is already used
		if _, exists := existingLinks[code]; !exists {
			return code, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique code after %d attempts", maxAttempts)
}

func generateRandomCode() (string, error) {
	b := make([]byte, codeLength)
	for i := range b {
		// Use crypto/rand for better randomness
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeChars))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		b[i] = codeChars[n.Int64()]
	}
	return string(b), nil
}

func validateURL(urlStr string) error {
	if len(urlStr) > maxURLLen {
		return fmt.Errorf("URL too long")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must be http or https")
	}

	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

func validateAlias(alias string) error {
	// Check reserved name
	if alias == "api" {
		return fmt.Errorf("alias cannot be 'api'")
	}

	// Check pattern: ^[A-Za-z0-9_-]{3,32}$
	if !aliasRegex.MatchString(alias) {
		return fmt.Errorf("alias must match ^[A-Za-z0-9_-]{3,32}$")
	}

	return nil
}