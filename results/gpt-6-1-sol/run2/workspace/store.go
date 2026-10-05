package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var (
	errConflict = errors.New("alias already taken")
	errNotFound = errors.New("link not found")
)

// Link is the on-disk and API representation of a shortened URL.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    uint64    `json:"visits"`
}

// Store serializes mutations and their durable writes under one lock. Readers
// cannot observe a change until its persistence attempt has completed.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]Link
}

func openStore(path string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, links: make(map[string]Link)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		_, err = s.saveLocked()
		return s, err
	}
	if err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}
	var links []Link
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	for _, link := range links {
		if !validAlias(link.Code) || !validURL(link.URL) || link.CreatedAt.IsZero() {
			return nil, fmt.Errorf("invalid stored link %q", link.Code)
		}
		if _, exists := s.links[link.Code]; exists {
			return nil, fmt.Errorf("duplicate stored code %q", link.Code)
		}
		s.links[link.Code] = link
	}
	return s, nil
}

func (s *Store) listLocked() []Link {
	links := make([]Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].CreatedAt.Equal(links[j].CreatedAt) {
			return links[i].Code < links[j].Code
		}
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})
	return links
}

func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

// saveLocked uses a temporary file in the same directory, fsync, and an atomic
// rename. The data file is always either the complete old or complete new JSON.
// committed distinguishes a pre-rename failure (safe to roll back memory) from
// a directory-sync failure after the new file has already become visible.
func (s *Store) saveLocked() (committed bool, err error) {
	data, err := json.Marshal(s.listLocked())
	if err != nil {
		return false, err
	}
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return false, err
	}
	defer dir.Close()
	f, err := os.CreateTemp(filepath.Dir(s.path), ".shortener-*.tmp")
	if err != nil {
		return false, err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	if _, err = f.Write(append(data, '\n')); err != nil {
		return false, err
	}
	if err = f.Sync(); err != nil {
		return false, err
	}
	if err = f.Close(); err != nil {
		return false, err
	}
	if err = os.Rename(name, s.path); err != nil {
		return false, err
	}
	return true, dir.Sync()
}

func randomCode() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var code [7]byte
	for i := range code {
		for {
			var b [1]byte
			if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
				return "", err
			}
			// Rejection sampling avoids modulo bias.
			if b[0] < 248 {
				code[i] = alphabet[int(b[0])%len(alphabet)]
				break
			}
		}
	}
	return string(code[:]), nil
}

func (s *Store) Create(url, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code != "" {
		if _, exists := s.links[code]; exists {
			return Link{}, errConflict
		}
	} else {
		for {
			var err error
			code, err = randomCode()
			if err != nil {
				return Link{}, err
			}
			if _, exists := s.links[code]; !exists {
				break
			}
		}
	}
	link := Link{Code: code, URL: url, CreatedAt: time.Now().UTC()}
	s.links[code] = link
	if committed, err := s.saveLocked(); err != nil {
		if !committed {
			delete(s.links, code)
		}
		return Link{}, err
	}
	return link, nil
}

func (s *Store) Visit(code string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.links[code]
	if !ok {
		return Link{}, errNotFound
	}
	if old.Visits == math.MaxUint64 {
		return Link{}, errors.New("visit counter exhausted")
	}
	link := old
	link.Visits++
	s.links[code] = link
	if committed, err := s.saveLocked(); err != nil {
		if !committed {
			s.links[code] = old
		}
		return Link{}, err
	}
	return link, nil
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.links[code]
	if !ok {
		return errNotFound
	}
	delete(s.links, code)
	if committed, err := s.saveLocked(); err != nil {
		if !committed {
			s.links[code] = old
		}
		return err
	}
	return nil
}
