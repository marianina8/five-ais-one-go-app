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
	Code      string `json:"code"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	Visits    int64  `json:"visits"`
}

var ErrExists = errors.New("alias already taken")
var ErrNotFound = errors.New("not found")

type Store struct {
	mu    sync.Mutex
	path  string
	links []*Link
	index map[string]*Link
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, index: map[string]*Link{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	var ls []*Link
	if err := json.Unmarshal(b, &ls); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, l := range ls {
		if l == nil || l.Code == "" {
			continue
		}
		if _, dup := s.index[l.Code]; dup {
			continue
		}
		s.links = append(s.links, l)
		s.index[l.Code] = l
	}
	return s, nil
}

// save writes atomically: temp file, fsync, rename, fsync dir. Caller holds mu.
func (s *Store) save() error {
	data, err := json.Marshal(s.links)
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
	if _, err := f.Write(data); err != nil {
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
	var b [7]byte
	max := byte(256 - 256%len(alphabet))
	out := make([]byte, 0, 7)
	for len(out) < 7 {
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		for _, c := range b {
			if c < max && len(out) < 7 {
				out = append(out, alphabet[int(c)%len(alphabet)])
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
		if _, ok := s.index[code]; ok {
			return Link{}, ErrExists
		}
	} else {
		for {
			c, err := randomCode()
			if err != nil {
				return Link{}, err
			}
			if _, ok := s.index[c]; !ok {
				code = c
				break
			}
		}
	}
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.links = append(s.links, l)
	s.index[code] = l
	if err := s.save(); err != nil {
		s.links = s.links[:len(s.links)-1]
		delete(s.index, code)
		return Link{}, err
	}
	return *l, nil
}

// Visit increments the visit counter and returns the URL.
func (s *Store) Visit(code string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.index[code]
	if !ok {
		return "", ErrNotFound
	}
	l.Visits++
	if err := s.save(); err != nil {
		l.Visits--
		return "", err
	}
	return l.URL, nil
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.index[code]
	if !ok {
		return ErrNotFound
	}
	pos := -1
	for i, x := range s.links {
		if x == l {
			pos = i
			break
		}
	}
	old := append([]*Link(nil), s.links...)
	s.links = append(s.links[:pos:pos], s.links[pos+1:]...)
	delete(s.index, code)
	if err := s.save(); err != nil {
		s.links = old
		s.index[code] = l
		return err
	}
	return nil
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
