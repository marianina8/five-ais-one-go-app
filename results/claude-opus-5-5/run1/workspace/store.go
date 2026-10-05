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
	errTaken    = errors.New("alias already taken")
	errNotFound = errors.New("not found")
)

// Store keeps links in memory and persists them atomically to a JSON file.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	order []string // codes, oldest first
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]*Link{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	var list []Link
	if len(data) > 0 {
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
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

// saveLocked writes all links to a temp file, fsyncs and renames it into place.
func (s *Store) saveLocked() error {
	list := make([]Link, 0, len(s.order))
	for _, c := range s.order {
		list = append(list, *s.links[c])
	}
	data, err := json.MarshalIndent(list, "", "  ")
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
			os.Remove(tmp)
		}
	}()
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

func (s *Store) Create(url, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code != "" {
		if _, ok := s.links[code]; ok {
			return Link{}, errTaken
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
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC(), Visits: 0}
	s.links[code] = l
	s.order = append(s.order, code)
	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		s.order = s.order[:len(s.order)-1]
		return Link{}, err
	}
	return *l, nil
}

func (s *Store) Visit(code string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return "", errNotFound
	}
	l.Visits++
	if err := s.saveLocked(); err != nil {
		l.Visits--
		return "", err
	}
	return l.URL, nil
}

func (s *Store) Get(code string) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return Link{}, false
	}
	return *l, true
}

func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Link, 0, len(s.order))
	for _, c := range s.order {
		list = append(list, *s.links[c])
	}
	return list
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return errNotFound
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
