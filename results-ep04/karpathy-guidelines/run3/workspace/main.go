package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		fmt.Fprintln(os.Stderr, "error: ADMIN_TOKEN environment variable must be set")
		os.Exit(1)
	}

	store, err := NewStore(*dataFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to initialize store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	handler := NewHandler(store, adminToken)
	
	fmt.Printf("Starting server on %s\n", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "error: server failed: %v\n", err)
		os.Exit(1)
	}
}

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

type Store struct {
	mu    sync.RWMutex
	file  string
	links map[string]*Link
}

func NewStore(filename string) (*Store, error) {
	s := &Store{
		file:  filename,
		links: make(map[string]*Link),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var links []*Link
	if err := json.Unmarshal(data, &links); err != nil {
		return err
	}

	s.links = make(map[string]*Link)
	for _, link := range links {
		s.links[link.Code] = link
	}
	return nil
}

func (s *Store) saveLinks(links []*Link) error {
	data, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	tempFile := s.file + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tempFile, s.file)
}

func (s *Store) Create(code, url string) (*Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; exists {
		return nil, fmt.Errorf("code already exists")
	}

	link := &Link{
		Code:      code,
		URL:       url,
		CreatedAt: time.Now().UTC(),
		Visits:    0,
	}
	s.links[code] = link

	// Copy links for saving
	links := make([]*Link, 0, len(s.links))
	for _, l := range s.links {
		links = append(links, &Link{
			Code:      l.Code,
			URL:       l.URL,
			CreatedAt: l.CreatedAt,
			Visits:    l.Visits,
		})
	}

	if err := s.saveLinks(links); err != nil {
		delete(s.links, code)
		return nil, err
	}
	return link, nil
}

func (s *Store) Get(code string) (*Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link, exists := s.links[code]
	if !exists {
		return nil, fmt.Errorf("link not found")
	}
	return &Link{
		Code:      link.Code,
		URL:       link.URL,
		CreatedAt: link.CreatedAt,
		Visits:    link.Visits,
	}, nil
}

func (s *Store) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, exists := s.links[code]
	if !exists {
		return fmt.Errorf("link not found")
	}
	link.Visits++

	// Copy links for saving
	links := make([]*Link, 0, len(s.links))
	for _, l := range s.links {
		links = append(links, &Link{
			Code:      l.Code,
			URL:       l.URL,
			CreatedAt: l.CreatedAt,
			Visits:    l.Visits,
		})
	}

	return s.saveLinks(links)
}

func (s *Store) List() []*Link {
	s.mu.RLock()
	defer s.mu.RUnlock()

	links := make([]*Link, 0, len(s.links))
	for _, link := range s.links {
		links = append(links, &Link{
			Code:      link.Code,
			URL:       link.URL,
			CreatedAt: link.CreatedAt,
			Visits:    link.Visits,
		})
	}
	return links
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[code]; !exists {
		return fmt.Errorf("link not found")
	}
	delete(s.links, code)

	// Copy links for saving
	links := make([]*Link, 0, len(s.links))
	for _, l := range s.links {
		links = append(links, &Link{
			Code:      l.Code,
			URL:       l.URL,
			CreatedAt: l.CreatedAt,
			Visits:    l.Visits,
		})
	}

	return s.saveLinks(links)
}

func (s *Store) Close() error {
	return nil
}