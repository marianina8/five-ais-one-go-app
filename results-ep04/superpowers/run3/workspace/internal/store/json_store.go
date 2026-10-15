package store

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sync"
	"time"

	"shortener/internal/model"
)

type jsonStore struct {
	mu    sync.RWMutex
	links map[string]*model.Link
	path  string
}

func NewJsonStore(path string) *jsonStore {
	return &jsonStore{
		links: make(map[string]*model.Link),
		path:  path,
	}
}

func (s *jsonStore) Create(url, alias string) (*model.Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check alias if provided
	if alias != "" {
		if _, exists := s.links[alias]; exists {
			return nil, fmt.Errorf("alias already taken")
		}
	}
	
	code := alias
	if code == "" {
		// Generate unique random code
		for {
			code = generateRandomCode(7)
			if _, exists := s.links[code]; !exists {
				break
			}
		}
	} else if _, exists := s.links[code]; exists {
		return nil, fmt.Errorf("alias already taken")
	}
	
	link := &model.Link{
		URL:       url,
		Code:      code,
		Alias:     alias,
		CreatedAt: time.Now(),
		Visits:    0,
	}
	
	s.links[code] = link
	return link, nil
}

func (s *jsonStore) Get(code string) (*model.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	link, exists := s.links[code]
	if !exists {
		return nil, fmt.Errorf("link not found")
	}
	return link, nil
}

func (s *jsonStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	link, exists := s.links[code]
	if !exists {
		return fmt.Errorf("link not found")
	}
	
	link.Visits++
	return nil
}

func (s *jsonStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("link not found")
	}
	
	delete(s.links, code)
	return nil
}

func (s *jsonStore) List() []*model.Link {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	links := make([]*model.Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, link)
	}
	return links
}

func (s *jsonStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	tmpFile := s.path + ".tmp"
	data, err := json.MarshalIndent(s.links, "", "  ")
	if err != nil {
		return err
	}
	
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	
	return os.Rename(tmpFile, s.path)
}

func (s *jsonStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, start empty
		}
		return err
	}
	
	if err := json.Unmarshal(data, &s.links); err != nil {
		return err
	}
	
	return nil
}

func generateRandomCode(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback to deterministic if crypto fails
			b[i] = charset[i%len(charset)]
		} else {
			b[i] = charset[n.Int64()]
		}
	}
	return string(b)
}