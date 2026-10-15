package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"
)

type JSONStore struct {
	filepath string
	mu       sync.RWMutex
	links    map[string]Link
}

func NewJSONStore(filepath string) (*JSONStore, error) {
	store := &JSONStore{
		filepath: filepath,
		links:    make(map[string]Link),
	}

	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *JSONStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create empty file
			return s.save()
		}
		return err
	}
	defer file.Close()

	var links []Link
	if err := json.NewDecoder(file).Decode(&links); err != nil {
		// If file is empty or invalid JSON, start fresh
		if err == io.EOF {
			s.links = make(map[string]Link)
			return nil
		}
		return err
	}

	s.links = make(map[string]Link)
	for _, link := range links {
		s.links[link.Code] = link
	}
	return nil
}

func (s *JSONStore) save() error {
	tempFile := s.filepath + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer file.Close()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(links); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(tempFile, s.filepath)
}

var aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

func isValidAlias(alias string) bool {
	if alias == "" {
		return true // Empty alias means generate one
	}
	if alias == "api" {
		return false
	}
	return aliasRegex.MatchString(alias)
}

func (s *JSONStore) CreateLink(urlStr, alias string) (*Link, error) {
	// Validate URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("URL must use http or https scheme")
	}
	if u.Host == "" {
		return nil, errors.New("URL must have a host")
	}
	if len(urlStr) > 2048 {
		return nil, errors.New("URL too long")
	}

	// Validate alias
	if alias != "" && !isValidAlias(alias) {
		return nil, errors.New("invalid alias")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if alias already exists
	if alias != "" {
		for _, link := range s.links {
			if link.Alias == alias {
				return nil, errors.New("alias already taken")
			}
		}
	}

	// Generate code
	code := alias
	if code == "" {
		for {
			code = generateCode(7)
			if _, exists := s.links[code]; !exists {
				break
			}
		}
	} else {
		if _, exists := s.links[code]; exists {
			return nil, errors.New("code already exists")
		}
	}

	link := Link{
		Code:      code,
		URL:       urlStr,
		Alias:     alias,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link
	if err := s.save(); err != nil {
		// Remove the link if save fails
		delete(s.links, code)
		return nil, err
	}

	return &link, nil
}

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func generateCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func (s *JSONStore) GetLink(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, errors.New("link not found")
	}
	return &link, nil
}

func (s *JSONStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return errors.New("link not found")
	}

	link.Visits++
	s.links[code] = link
	return s.save()
}

func (s *JSONStore) ListLinks() ([]Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
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

func (s *JSONStore) DeleteLink(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return errors.New("link not found")
	}

	delete(s.links, code)
	return s.save()
}

func (s *JSONStore) Close() error {
	return nil
}