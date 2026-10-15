package main

import (
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
	mu    sync.RWMutex
	links map[string]Link
}

func newStore() *Store {
	return &Store{
		links: make(map[string]Link),
	}
}

func (s *Store) Close() {
	// Nothing to do for in-memory store
}

func (s *Store) Create(link Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[link.Code]; exists {
		return &StoreError{Code: "duplicate", Message: "alias already taken"}
	}

	s.links[link.Code] = link
	return nil
}

func (s *Store) Get(code string) (Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return Link{}, &StoreError{Code: "not_found", Message: "code not found"}
	}

	return link, nil
}

func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return &StoreError{Code: "not_found", Message: "code not found"}
	}

	link.Visits++
	s.links[code] = link
	return nil
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return &StoreError{Code: "not_found", Message: "code not found"}
	}

	delete(s.links, code)
	return nil
}

func (s *Store) List() []Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}

	// Sort by CreatedAt ascending (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links
}

type StoreError struct {
	Code    string
	Message string
}

func (e *StoreError) Error() string {
	return e.Message
}