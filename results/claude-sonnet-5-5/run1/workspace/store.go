package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Link struct {
	Code      string `json:"code"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	Visits    int64  `json:"visits"`
}

type fileFormat struct {
	Links []Link `json:"links"`
}

var (
	ErrExists   = errors.New("alias already taken")
	ErrNotFound = errors.New("not found")
)

type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	order []string
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]*Link{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	var ff fileFormat
	if err := json.Unmarshal(b, &ff); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for i := range ff.Links {
		l := ff.Links[i]
		if _, dup := s.links[l.Code]; dup {
			continue
		}
		s.links[l.Code] = &l
		s.order = append(s.order, l.Code)
	}
	return s, nil
}

// saveLocked atomically writes the whole state. Caller holds s.mu.
func (s *Store) saveLocked() error {
	ff := fileFormat{Links: make([]Link, 0, len(s.order))}
	for _, c := range s.order {
		ff.Links = append(ff.Links, *s.links[c])
	}
	b, err := json.Marshal(ff)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(b); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	ok = true
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomCode() (string, error) {
	b := make([]byte, 7)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

// Create adds a link. If alias is empty a random code is generated.
func (s *Store) Create(url, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code == "" {
		for {
			c, err := randomCode()
			if err != nil {
				return Link{}, err
			}
			if _, taken := s.links[c]; !taken && c != "api" {
				code = c
				break
			}
		}
	} else if _, taken := s.links[code]; taken {
		return Link{}, ErrExists
	}
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC().Format(time.RFC3339), Visits: 0}
	s.links[code] = l
	s.order = append(s.order, code)
	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		s.order = s.order[:len(s.order)-1]
		return Link{}, err
	}
	return *l, nil
}

// Visit increments the visit count, persists, and returns the URL.
func (s *Store) Visit(code string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return "", ErrNotFound
	}
	l.Visits++
	if err := s.saveLocked(); err != nil {
		l.Visits--
		return "", err
	}
	return l.URL, nil
}

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
	s.order = append(s.order[:idx], s.order[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		s.links[code] = l
		s.order = oldOrder
		return err
	}
	return nil
}

func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Link, 0, len(s.order))
	for _, c := range s.order {
		out = append(out, *s.links[c])
	}
	return out
}
