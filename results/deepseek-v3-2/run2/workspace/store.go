package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type Store interface {
	Create(urlStr, alias string) (*Link, error)
	Get(code string) (*Link, error)
	Visit(code string) error
	Delete(code string) error
	List() ([]Link, error)
}

type JSONStore struct {
	mu       sync.RWMutex
	filePath string
	links    map[string]*Link
}

func NewJSONStore(filePath string) (*JSONStore, error) {
	store := &JSONStore{
		filePath: filePath,
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

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return err
	}

	s.links = make(map[string]*Link)
	for i := range links {
		link := links[i]
		s.links[link.Code] = &link
	}

	return nil
}

func (s *JSONStore) save() error {
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, s.filePath)
}

func (s *JSONStore) Create(urlStr, alias string) (*Link, error) {
	if err := validateURL(urlStr); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var code string
	var err error

	if alias != "" {
		if err := validateAlias(alias); err != nil {
			return nil, err
		}
		if _, exists := s.links[alias]; exists {
			return nil, fmt.Errorf("alias already taken")
		}
		code = alias
	} else {
		code, err = generateUniqueCode(s.links)
		if err != nil {
			return nil, err
		}
	}

	link := &Link{
		Code:      code,
		URL:       urlStr,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}

	s.links[code] = link

	if err := s.save(); err != nil {
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
		return nil, fmt.Errorf("link not found")
	}
	return link, nil
}

func (s *JSONStore) Visit(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return fmt.Errorf("link not found")
	}

	link.Visits++

	if err := s.save(); err != nil {
		link.Visits--
		return err
	}

	return nil
}

func (s *JSONStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("link not found")
	}

	delete(s.links, code)

	if err := s.save(); err != nil {
		s.links[code] = &Link{Code: code}
		return err
	}

	return nil
}

func (s *JSONStore) List() ([]Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	return links, nil
}

func validateURL(urlStr string) error {
	if len(urlStr) > 2048 {
		return fmt.Errorf("URL must be at most 2048 characters")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must start with http or https")
	}

	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

func validateAlias(alias string) error {
	if alias == "api" {
		return fmt.Errorf("alias cannot be 'api'")
	}

	if len(alias) < 3 || len(alias) > 32 {
		return fmt.Errorf("alias must be between 3 and 32 characters")
	}

	for _, r := range alias {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("alias can only contain letters, numbers, underscores and hyphens")
		}
	}

	return nil
}