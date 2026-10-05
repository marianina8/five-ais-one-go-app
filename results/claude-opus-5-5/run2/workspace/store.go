package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Link is a stored short link.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

var (
	ErrExists   = errors.New("alias already taken")
	ErrNotFound = errors.New("link not found")
)

// Store keeps links in memory and persists every change atomically to a JSON file.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	order []string // codes, oldest first
}

// OpenStore loads the data file (if present).
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]*Link{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	var list []Link
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	for i := range list {
		l := list[i]
		if _, dup := s.links[l.Code]; dup {
			continue
		}
		s.links[l.Code] = &l
		s.order = append(s.order, l.Code)
	}
	return s, nil
}

func (s *Store) snapshotLocked() []Link {
	out := make([]Link, 0, len(s.order))
	for _, c := range s.order {
		out = append(out, *s.links[c])
	}
	return out
}

// saveLocked writes the data to a temp file, fsyncs it and renames it over the target.
func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.snapshotLocked(), "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".shortener-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { f.Close(); os.Remove(tmp) }
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

const codeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomCode() (string, error) {
	b := make([]byte, 7)
	max := big.NewInt(int64(len(codeChars)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = codeChars[n.Int64()]
	}
	return string(b), nil
}

// Create adds a link. If alias is empty, a random unique code is generated.
func (s *Store) Create(url, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code != "" {
		if _, ok := s.links[code]; ok {
			return Link{}, ErrExists
		}
	} else {
		for {
			c, err := randomCode()
			if err != nil {
				return Link{}, err
			}
			if _, ok := s.links[c]; !ok && c != "api" {
				code = c
				break
			}
		}
	}
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC().Truncate(time.Second), Visits: 0}
	s.links[code] = l
	s.order = append(s.order, code)
	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		s.order = s.order[:len(s.order)-1]
		return Link{}, err
	}
	return *l, nil
}

// Visit increments the visit count and returns the link.
func (s *Store) Visit(code string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	l.Visits++
	if err := s.saveLocked(); err != nil {
		l.Visits--
		return Link{}, err
	}
	return *l, nil
}

// Get returns a link without modifying it.
func (s *Store) Get(code string) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return Link{}, false
	}
	return *l, true
}

// List returns all links, oldest first.
func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

// Delete removes a link.
func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return ErrNotFound
	}
	idx := -1
	for i, c := range s.order {
		if c == code {
			idx = i
			break
		}
	}
	oldOrder := append([]string(nil), s.order...)
	delete(s.links, code)
	if idx >= 0 {
		s.order = append(s.order[:idx], s.order[idx+1:]...)
	}
	if err := s.saveLocked(); err != nil {
		s.links[code] = l
		s.order = oldOrder
		return err
	}
	return nil
}
