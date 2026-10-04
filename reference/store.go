package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	codeLength   = 7
	codeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// ErrAliasTaken is returned when a link with the requested alias already exists.
var ErrAliasTaken = errors.New("alias already taken")

// Link is one shortened URL.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

// Store keeps every link in memory and writes the whole set to a JSON file after each change.
// One mutex guards both, so a change and its save happen together: when a method returns
// without an error, the change is on disk.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	order []string // codes, oldest first
}

// OpenStore loads the links saved at path. A missing file means an empty store.
func OpenStore(path string) (*Store, error) {
	store := &Store{path: path, links: map[string]*Link{}}
	data, err := os.ReadFile(path) // #nosec G304 -- the path is the operator's -data flag
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	var saved []*Link
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	for _, link := range saved {
		store.links[link.Code] = link
		store.order = append(store.order, link.Code)
	}
	return store, nil
}

// Create adds a link. An empty alias means: generate a random code.
func (s *Store) Create(url, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	code := alias
	if code == "" {
		var err error
		if code, err = s.unusedCodeLocked(); err != nil {
			return Link{}, err
		}
	} else if _, exists := s.links[code]; exists {
		return Link{}, ErrAliasTaken
	}

	link := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC()}
	previousOrder := s.order
	s.links[code] = link
	// Clip makes append copy, so previousOrder is untouched if the save fails.
	s.order = append(slices.Clip(s.order), code)
	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		s.order = previousOrder
		return Link{}, err
	}
	return *link, nil
}

// Visit counts a visit and returns the link's URL. found is false for an unknown code.
func (s *Store) Visit(code string) (url string, found bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, found := s.links[code]
	if !found {
		return "", false, nil
	}
	link.Visits++
	if err := s.saveLocked(); err != nil {
		link.Visits--
		return "", true, err
	}
	return link.URL, true, nil
}

// List returns a copy of every link, oldest first.
func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

// Delete removes a link. found is false for an unknown code.
func (s *Store) Delete(code string) (found bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, found := s.links[code]
	if !found {
		return false, nil
	}
	previousOrder := s.order
	index := slices.Index(s.order, code)
	s.order = slices.Delete(slices.Clone(s.order), index, index+1)
	delete(s.links, code)
	if err := s.saveLocked(); err != nil {
		s.links[code] = link
		s.order = previousOrder
		return true, err
	}
	return true, nil
}

func (s *Store) snapshotLocked() []Link {
	snapshot := make([]Link, 0, len(s.order)) // never nil: an empty store lists as []
	for _, code := range s.order {
		snapshot = append(snapshot, *s.links[code])
	}
	return snapshot
}

func (s *Store) saveLocked() error {
	data, err := json.Marshal(s.snapshotLocked())
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data)
}

// unusedCodeLocked returns a random code no link uses yet.
func (s *Store) unusedCodeLocked() (string, error) {
	for {
		code, err := randomCode()
		if err != nil {
			return "", err
		}
		if _, taken := s.links[code]; !taken {
			return code, nil
		}
	}
}

func randomCode() (string, error) {
	code := make([]byte, codeLength)
	alphabetSize := big.NewInt(int64(len(codeAlphabet)))
	for i := range code {
		n, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", err
		}
		code[i] = codeAlphabet[n.Int64()]
	}
	return string(code), nil
}

// writeFileAtomic replaces path with data so that a crash at any moment leaves either
// the old file or the new one, never a half-written file: write a temporary file in the
// same folder, flush it to disk, then rename it over the old one.
func writeFileAtomic(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
