package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Store interface {
	Create(link Link) error
	Get(code string) (Link, bool)
	GetAll() []Link
	IncrementVisits(code string) error
	Delete(code string) error
}

type fileStore struct {
	mu   sync.RWMutex
	path string
	data map[string]Link
}

func NewFileStore(path string) (Store, error) {
	fs := &fileStore{
		path: path,
		data: make(map[string]Link),
	}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (fs *fileStore) close() {
	// Nothing to do for now
}

func (fs *fileStore) load() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Check if file exists
	file, err := os.Open(fs.path)
	if os.IsNotExist(err) {
		// File doesn't exist, that's OK - we'll create it on first save
		return nil
	}
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var links []Link
	if err := decoder.Decode(&links); err != nil {
		if err == io.EOF {
			// Empty file is OK
			return nil
		}
		return fmt.Errorf("decode JSON: %w", err)
	}

	// Convert slice to map
	for _, link := range links {
		fs.data[link.Code] = link
	}

	return nil
}

func (fs *fileStore) save() error {
	// Create a temporary file
	tempFile, err := os.CreateTemp(filepath.Dir(fs.path), filepath.Base(fs.path)+".tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Convert map to slice for JSON
	links := make([]Link, 0, len(fs.data))
	for _, link := range fs.data {
		links = append(links, link)
	}

	// Write JSON
	encoder := json.NewEncoder(tempFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(links); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	// Flush to disk
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	tempFile.Close()

	// Atomic rename
	if err := os.Rename(tempFile.Name(), fs.path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func (fs *fileStore) Create(link Link) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.data[link.Code]; exists {
		return fmt.Errorf("code already exists")
	}
	fs.data[link.Code] = link
	return fs.save()
}

func (fs *fileStore) Get(code string) (Link, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	link, exists := fs.data[code]
	return link, exists
}

func (fs *fileStore) GetAll() []Link {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	links := make([]Link, 0, len(fs.data))
	for _, link := range fs.data {
		links = append(links, link)
	}

	// Sort by CreatedAt ascending (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	return links
}

func (fs *fileStore) IncrementVisits(code string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	link, exists := fs.data[code]
	if !exists {
		return fmt.Errorf("code not found")
	}
	link.Visits++
	fs.data[code] = link
	return fs.save()
}

func (fs *fileStore) Delete(code string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.data[code]; !exists {
		return fmt.Errorf("code not found")
	}
	delete(fs.data, code)
	return fs.save()
}