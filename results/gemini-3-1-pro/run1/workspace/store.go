package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
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
	mu       sync.Mutex
	filePath string
	links    []*Link
	linkMap  map[string]*Link
}

func NewStore(filePath string) (*Store, error) {
	s := &Store{
		filePath: filePath,
		links:    make([]*Link, 0),
		linkMap:  make(map[string]*Link),
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read file: %w", err)
		}
	} else {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &s.links); err != nil {
				return nil, fmt.Errorf("unmarshal data: %w", err)
			}
			for _, l := range s.links {
				s.linkMap[l.Code] = l
			}
		}
	}

	return s, nil
}

func (s *Store) saveLocked() error {
	data, err := json.Marshal(s.links)
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	f, err := os.CreateTemp(dir, "data-*.json")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer os.Remove(tmpName) // cleans up if not renamed

	if _, err := f.Write(data); err != nil {
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

	return os.Rename(tmpName, s.filePath)
}

func (s *Store) Create(url string, alias string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var code string
	if alias != "" {
		if _, exists := s.linkMap[alias]; exists {
			return nil, fmt.Errorf("conflict")
		}
		code = alias
	} else {
		for {
			code = generateRandomCode()
			if _, exists := s.linkMap[code]; !exists {
				break
			}
		}
	}

	l := &Link{
		Code:      code,
		URL:       url,
		CreatedAt: time.Now().Truncate(time.Millisecond).UTC(),
		Visits:    0,
	}

	s.links = append(s.links, l)
	s.linkMap[l.Code] = l

	if err := s.saveLocked(); err != nil {
		// Rollback in memory
		s.links = s.links[:len(s.links)-1]
		delete(s.linkMap, l.Code)
		return nil, err
	}

	return l, nil
}

func (s *Store) GetAndVisit(code string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, exists := s.linkMap[code]
	if !exists {
		return nil, fmt.Errorf("not found")
	}

	l.Visits++
	if err := s.saveLocked(); err != nil {
		l.Visits--
		return nil, err
	}

	return l, nil
}

func (s *Store) List() []*Link {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Return a copy to avoid data races
	res := make([]*Link, len(s.links))
	for i, l := range s.links {
		copied := *l
		res[i] = &copied
	}
	return res
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.linkMap[code]; !exists {
		return fmt.Errorf("not found")
	}

	// Remove from slice
	idx := -1
	for i, l := range s.links {
		if l.Code == code {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("not found in slice") // Should not happen
	}

	deletedLink := s.links[idx]

	// Slice removal preserving order
	newLinks := make([]*Link, 0, len(s.links)-1)
	newLinks = append(newLinks, s.links[:idx]...)
	newLinks = append(newLinks, s.links[idx+1:]...)

	s.links = newLinks
	delete(s.linkMap, code)

	if err := s.saveLocked(); err != nil {
		// Rollback
		s.links = append(s.links, nil)
		copy(s.links[idx+1:], s.links[idx:])
		s.links[idx] = deletedLink
		s.linkMap[code] = deletedLink
		return err
	}

	return nil
}

func generateRandomCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 7)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
