package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sync"
	"time"
)

var (
	aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type Store struct {
	mu     sync.RWMutex
	file   string
	links  map[string]*Link
	byCode map[string]*Link
}

func NewStore(file string) (*Store, error) {
	s := &Store{
		file:   file,
		links:  make(map[string]*Link),
		byCode: make(map[string]*Link),
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var links []*Link
	if err := json.NewDecoder(f).Decode(&links); err != nil {
		return err
	}

	s.links = make(map[string]*Link)
	s.byCode = make(map[string]*Link)

	for _, link := range links {
		s.links[link.URL] = link
		s.byCode[link.Code] = link
	}

	return nil
}

func (s *Store) save() error {
	links := make([]*Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.file + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, s.file)
}

func (s *Store) generateCode() (string, error) {
	// Try at most 100 times
	for i := 0; i < 100; i++ {
		code, err := randomCode(7)
		if err != nil {
			return "", err
		}
		if _, exists := s.byCode[code]; !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique code after 100 attempts")
}

func (s *Store) CreateLink(urlStr, alias string) (*Link, error) {
	// Validate URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
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

	s.mu.Lock()
	defer s.mu.Unlock()

	// Determine code
	var code string
	if alias != "" {
		if alias == "api" {
			return nil, fmt.Errorf("alias 'api' is reserved")
		}
		if !aliasRegex.MatchString(alias) {
			return nil, fmt.Errorf("alias must match ^[A-Za-z0-9_-]{3,32}$")
		}
		code = alias
	} else {
		var err error
		code, err = s.generateCode()
		if err != nil {
			return nil, fmt.Errorf("failed to generate unique code: %w", err)
		}
	}

	// Check if code already exists
	if _, exists := s.byCode[code]; exists {
		return nil, fmt.Errorf("code '%s' already exists", code)
	}

	link := &Link{
		Code:      code,
		URL:       urlStr,
		CreatedAt: time.Now(),
		Visits:    0,
	}

	s.links[urlStr] = link
	s.byCode[code] = link

	if err := s.save(); err != nil {
		// Rollback
		delete(s.links, urlStr)
		delete(s.byCode, code)
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return link, nil
}

func (s *Store) GetLink(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.byCode[code]
	if !exists {
		return nil, fmt.Errorf("code '%s' not found", code)
	}
	return link, nil
}

func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.byCode[code]
	if !exists {
		return fmt.Errorf("code '%s' not found", code)
	}

	link.Visits++

	if err := s.save(); err != nil {
		link.Visits-- // Rollback
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *Store) ListLinks() []*Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}
	return links
}

func (s *Store) DeleteLink(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.byCode[code]
	if !exists {
		return fmt.Errorf("code '%s' not found", code)
	}

	delete(s.byCode, code)
	delete(s.links, link.URL)

	if err := s.save(); err != nil {
		// Rollback
		s.byCode[code] = link
		s.links[link.URL] = link
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	return nil // Nothing to close
}