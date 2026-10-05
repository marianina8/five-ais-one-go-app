package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound  = errors.New("link not found")
	ErrConflict  = errors.New("alias already taken")
	aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
)

// Link is both the public API representation and the on-disk record.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    uint64    `json:"visits"`
}

// Store serializes entire read-modify-save transactions. Readers see only
// committed state. It is intended to be owned by a single service process.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]Link
}

func validURL(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 2048 {
		return false
	}
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Hostname() != ""
}

func validAlias(value string) bool {
	return value != "api" && aliasPattern.MatchString(value)
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, links: make(map[string]Link)}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var links []Link
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&links); err != nil {
		return nil, fmt.Errorf("invalid storage: %w", err)
	}
	if err := requireEOF(dec); err != nil {
		return nil, fmt.Errorf("invalid storage: %w", err)
	}
	if links == nil {
		return nil, errors.New("invalid storage: expected a JSON array")
	}
	for _, link := range links {
		if !validAlias(link.Code) || !validURL(link.URL) || link.CreatedAt.IsZero() {
			return nil, errors.New("invalid storage: invalid link")
		}
		if _, exists := s.links[link.Code]; exists {
			return nil, errors.New("invalid storage: duplicate code")
		}
		s.links[link.Code] = link
	}
	return s, nil
}

func requireEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func sortedLinks(links map[string]Link) []Link {
	result := make([]Link, 0, len(links))
	for _, link := range links {
		result = append(result, link)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].Code < result[j].Code
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// atomicSave never truncates the live file. The temporary file is flushed
// before rename, and the parent directory is flushed afterward. A kill leaves
// either the previous complete snapshot or the new complete snapshot.
// committed indicates that rename succeeded, even if the final directory sync
// failed, so the in-memory state cannot diverge from the visible disk state.
func atomicSave(path string, links map[string]Link) (committed bool, err error) {
	data, err := json.Marshal(sortedLinks(links))
	if err != nil {
		return false, err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return false, err
	}
	defer dir.Close()
	f, err := os.CreateTemp(filepath.Dir(path), ".shortener-*.tmp")
	if err != nil {
		return false, err
	}
	defer os.Remove(f.Name())
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
	if err = os.Rename(f.Name(), path); err != nil {
		return false, err
	}
	return true, dir.Sync()
}

func (s *Store) snapshot() map[string]Link {
	links := make(map[string]Link, len(s.links))
	for code, link := range s.links {
		links[code] = link
	}
	return links
}

func (s *Store) commit(links map[string]Link) error {
	committed, err := atomicSave(s.path, links)
	if committed {
		s.links = links
	}
	return err
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomCode() (string, error) {
	var b strings.Builder
	// Rejection sampling avoids the bias from byte % 62.
	for b.Len() < 7 {
		var sample [16]byte
		if _, err := rand.Read(sample[:]); err != nil {
			return "", err
		}
		for _, v := range sample {
			if v < 248 {
				b.WriteByte(alphabet[int(v)%len(alphabet)])
				if b.Len() == 7 {
					break
				}
			}
		}
	}
	return b.String(), nil
}

func (s *Store) Create(target string, alias *string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validURL(target) {
		return Link{}, errors.New("invalid URL")
	}
	var code string
	if alias != nil {
		if !validAlias(*alias) {
			return Link{}, errors.New("invalid alias")
		}
		code = *alias
		if _, exists := s.links[code]; exists {
			return Link{}, ErrConflict
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
	link := Link{Code: code, URL: target, CreatedAt: time.Now().UTC()}
	links := s.snapshot()
	links[code] = link
	if err := s.commit(links); err != nil {
		return Link{}, err
	}
	return link, nil
}

func (s *Store) Follow(code string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	link, exists := s.links[code]
	if !exists {
		return Link{}, ErrNotFound
	}
	if link.Visits == ^uint64(0) {
		return Link{}, errors.New("visit counter exhausted")
	}
	link.Visits++
	links := s.snapshot()
	links[code] = link
	if err := s.commit(links); err != nil {
		return Link{}, err
	}
	return link, nil
}

func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedLinks(s.links)
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.links[code]; !exists {
		return ErrNotFound
	}
	links := s.snapshot()
	delete(links, code)
	return s.commit(links)
}
