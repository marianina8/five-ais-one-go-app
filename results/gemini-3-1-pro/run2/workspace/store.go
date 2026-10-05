package main

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

var ErrDuplicateCode = errors.New("duplicate code")

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	links  []*Link
	byCode map[string]*Link
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path:   path,
		links:  make([]*Link, 0),
		byCode: make(map[string]*Link),
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}

	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.links); err != nil {
			return nil, err
		}
		for _, l := range s.links {
			s.byCode[l.Code] = l
		}
	}
	return s, nil
}

// save must be called with mu held.
func (s *Store) save() error {
	b, err := json.Marshal(s.links)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Atomic rename ensures crash safety
	return os.Rename(tmp, s.path)
}

func (s *Store) Create(code, url string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byCode[code]; exists {
		return nil, ErrDuplicateCode
	}

	l := &Link{
		Code:      code,
		URL:       url,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}
	s.links = append(s.links, l)
	s.byCode[code] = l

	if err := s.save(); err != nil {
		// Revert on failure
		s.links = s.links[:len(s.links)-1]
		delete(s.byCode, code)
		return nil, err
	}
	return l, nil
}

func (s *Store) Visit(code string) (*Link, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.byCode[code]
	if !ok {
		return nil, false, nil
	}

	l.Visits++
	if err := s.save(); err != nil {
		l.Visits--
		return nil, true, err
	}
	return l, true, nil
}

func (s *Store) Delete(code string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.byCode[code]
	if !ok {
		return false, nil
	}

	// Remove from slice
	idx := -1
	for i, l := range s.links {
		if l.Code == code {
			idx = i
			break
		}
	}
	if idx != -1 {
		s.links = append(s.links[:idx], s.links[idx+1:]...)
	}
	
	// Keep a copy of the old link in case we need to revert
	oldLink := s.byCode[code]
	delete(s.byCode, code)

	if err := s.save(); err != nil {
		// Revert
		s.byCode[code] = oldLink
		if idx != -1 {
			s.links = append(s.links[:idx], append([]*Link{oldLink}, s.links[idx:]...)...)
		}
		return true, err
	}
	return true, nil
}

func (s *Store) GetByCode(code string) (*Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.byCode[code]
	return l, ok
}

func (s *Store) List() []*Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]*Link, len(s.links))
	copy(res, s.links)
	return res
}
