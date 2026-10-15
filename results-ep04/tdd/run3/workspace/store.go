package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type Store struct {
	mu       sync.RWMutex
	filePath string
	links    map[string]*Link
}

func NewStore(filePath string) (*Store, error) {
	s := &Store{
		filePath: filePath,
		links:    make(map[string]*Link),
	}

	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load store: %w", err)
	}

	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var links []Link
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&links); err != nil && err != io.EOF {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	s.links = make(map[string]*Link)
	for i := range links {
		link := &links[i]
		s.links[link.Code] = link
	}

	return nil
}

func (s *Store) save() error {
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	file, err := os.Create(s.filePath + ".tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(file.Name())

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(links); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	file.Close()

	if err := os.Rename(file.Name(), s.filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (s *Store) Add(url, alias string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if alias != "" {
		if _, exists := s.links[alias]; exists {
			return nil, &ConflictError{Code: alias}
		}
		code := alias
		link := &Link{
			Code:      code,
			URL:       url,
			CreatedAt: time.Now(),
			Visits:    0,
		}
		s.links[code] = link
		if err := s.save(); err != nil {
			delete(s.links, code)
			return nil, fmt.Errorf("failed to save: %w", err)
		}
		return link, nil
	}

	// Generate random code
	for i := 0; i < 1000; i++ {
		code := randomCode(7)
		if _, exists := s.links[code]; !exists && code != "api" {
			link := &Link{
				Code:      code,
				URL:       url,
				CreatedAt: time.Now(),
				Visits:    0,
			}
			s.links[code] = link
			if err := s.save(); err != nil {
				delete(s.links, code)
				return nil, fmt.Errorf("failed to save: %w", err)
			}
			return link, nil
		}
	}

	return nil, fmt.Errorf("failed to generate unique code")
}

func (s *Store) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, &NotFoundError{Code: code}
	}

	return link, nil
}

func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return &NotFoundError{Code: code}
	}

	link.Visits++
	if err := s.save(); err != nil {
		link.Visits--
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *Store) List() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, *link)
	}

	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return &NotFoundError{Code: code}
	}

	delete(s.links, code)
	if err := s.save(); err != nil {
		s.links[code] = &Link{Code: code}
		return fmt.Errorf("failed to save: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	return nil
}

type NotFoundError struct {
	Code string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("link %s not found", e.Code)
}

type ConflictError struct {
	Code string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("code %s already taken", e.Code)
}