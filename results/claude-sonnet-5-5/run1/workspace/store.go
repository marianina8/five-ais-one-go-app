package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
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

var ErrTaken = errors.New("alias already taken")
var ErrNotFound = errors.New("not found")

type Store struct {
	mu    sync.Mutex
	path  string
	links []*Link
	byKey map[string]*Link
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, byKey: map[string]*Link{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	var ls []*Link
	if err := json.Unmarshal(b, &ls); err != nil {
		return nil, err
	}
	for _, l := range ls {
		if l == nil {
			continue
		}
		s.links = append(s.links, l)
		s.byKey[l.Code] = l
	}
	return s, nil
}

// saveLocked writes atomically: temp file, fsync, rename.
func (s *Store) saveLocked() error {
	b, err := json.Marshal(s.links)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".data-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(name)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(name)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		os.Remove(name)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomCode() (string, error) {
	b := make([]byte, 7)
	for i := range b {
		for {
			var x [1]byte
			if _, err := rand.Read(x[:]); err != nil {
				return "", err
			}
			if x[0] < 248 { // 248 = 62*4
				b[i] = alphabet[int(x[0])%62]
				break
			}
		}
	}
	return string(b), nil
}

func (s *Store) Create(url, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code != "" {
		if _, ok := s.byKey[code]; ok {
			return Link{}, ErrTaken
		}
	} else {
		for {
			c, err := randomCode()
			if err != nil {
				return Link{}, err
			}
			if _, ok := s.byKey[c]; !ok {
				code = c
				break
			}
		}
	}
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.links = append(s.links, l)
	s.byKey[code] = l
	if err := s.saveLocked(); err != nil {
		s.links = s.links[:len(s.links)-1]
		delete(s.byKey, code)
		return Link{}, err
	}
	return *l, nil
}

// Lookup returns a link without counting a visit.
func (s *Store) Lookup(code string) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.byKey[code]
	if !ok {
		return Link{}, false
	}
	return *l, true
}

func (s *Store) Visit(code string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.byKey[code]
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

func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Link, 0, len(s.links))
	for _, l := range s.links {
		out = append(out, *l)
	}
	return out
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.byKey[code]
	if !ok {
		return ErrNotFound
	}
	idx := -1
	for i, x := range s.links {
		if x == l {
			idx = i
			break
		}
	}
	old := s.links
	nl := make([]*Link, 0, len(old)-1)
	nl = append(nl, old[:idx]...)
	nl = append(nl, old[idx+1:]...)
	s.links = nl
	delete(s.byKey, code)
	if err := s.saveLocked(); err != nil {
		s.links = old
		s.byKey[code] = l
		return err
	}
	return nil
}
