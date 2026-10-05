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

// Link is a stored short link.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

var (
	errExists   = errors.New("alias already taken")
	errNotFound = errors.New("not found")
)

// Store keeps links in memory and persists them atomically to a JSON file.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	order []string
}

// OpenStore loads the store from path (missing file means empty store).
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]*Link{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	var list []*Link
	if len(data) > 0 {
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
	}
	for _, l := range list {
		if l == nil || l.Code == "" {
			continue
		}
		if _, dup := s.links[l.Code]; dup {
			continue
		}
		s.links[l.Code] = l
		s.order = append(s.order, l.Code)
	}
	return s, nil
}

// save writes the state atomically. Caller holds mu.
func (s *Store) save() error {
	list := make([]*Link, 0, len(s.order))
	for _, c := range s.order {
		list = append(list, s.links[c])
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(s.path)+"-*")
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

const codeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomCode() string {
	out := make([]byte, 0, 7)
	buf := make([]byte, 16)
	for len(out) < 7 {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		for _, b := range buf {
			if b < 248 && len(out) < 7 { // 248 = 62*4, avoids modulo bias
				out = append(out, codeChars[int(b)%62])
			}
		}
	}
	return string(out)
}

// Create adds a link. Empty alias means generate a code.
func (s *Store) Create(rawURL, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code == "" {
		for {
			code = randomCode()
			if _, taken := s.links[code]; !taken {
				break
			}
		}
	} else if _, taken := s.links[code]; taken {
		return Link{}, errExists
	}
	l := &Link{Code: code, URL: rawURL, CreatedAt: time.Now().UTC(), Visits: 0}
	s.links[code] = l
	s.order = append(s.order, code)
	if err := s.save(); err != nil {
		delete(s.links, code)
		s.order = s.order[:len(s.order)-1]
		return Link{}, err
	}
	return *l, nil
}

// Visit increments the visit count and returns the URL.
func (s *Store) Visit(code string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return "", errNotFound
	}
	l.Visits++
	if err := s.save(); err != nil {
		l.Visits--
		return "", err
	}
	return l.URL, nil
}

// List returns all links oldest first.
func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Link, 0, len(s.order))
	for _, c := range s.order {
		out = append(out, *s.links[c])
	}
	return out
}

// Delete removes a link.
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
	oldOrder := s.order
	newOrder := make([]string, 0, len(s.order))
	newOrder = append(newOrder, s.order[:idx]...)
	newOrder = append(newOrder, s.order[idx+1:]...)
	delete(s.links, code)
	s.order = newOrder
	if err := s.save(); err != nil {
		s.links[code] = l
		s.order = oldOrder
		return err
	}
	return nil
}
