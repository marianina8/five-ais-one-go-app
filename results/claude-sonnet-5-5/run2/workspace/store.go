package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

var (
	ErrExists   = errors.New("alias already taken")
	ErrNotFound = errors.New("not found")
)

type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	order []*Link
}

type fileFormat struct {
	Links []*Link `json:"links"`
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
	for _, l := range ff.Links {
		if l == nil || l.Code == "" {
			continue
		}
		if _, dup := s.links[l.Code]; dup {
			continue
		}
		s.links[l.Code] = l
		s.order = append(s.order, l)
	}
	return s, nil
}

// saveLocked writes the whole state atomically. Caller holds s.mu.
func (s *Store) saveLocked() error {
	ff := fileFormat{Links: s.order}
	if ff.Links == nil {
		ff.Links = []*Link{}
	}
	b, err := json.Marshal(ff)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".shortener-*.tmp")
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
	out := make([]byte, 0, 7)
	buf := make([]byte, 16)
	for len(out) < 7 {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, c := range buf {
			if c >= 248 { // 248 = 62*4, avoid modulo bias
				continue
			}
			out = append(out, alphabet[int(c)%62])
			if len(out) == 7 {
				break
			}
		}
	}
	return string(out), nil
}

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
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	s.links[code] = l
	s.order = append(s.order, l)
	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		s.order = s.order[:len(s.order)-1]
		return Link{}, err
	}
	return *l, nil
}

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

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return ErrNotFound
	}
	idx := -1
	for i, x := range s.order {
		if x == l {
			idx = i
			break
		}
	}
	oldOrder := append([]*Link(nil), s.order...)
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
	for _, l := range s.order {
		out = append(out, *l)
	}
	return out
}
