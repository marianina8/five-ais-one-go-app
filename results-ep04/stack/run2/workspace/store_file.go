package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type FileStore struct {
	*MemoryStore
	filename string
	mu       sync.Mutex
}

type fileStoreData struct {
	Links []*Link `json:"links"`
}

func NewFileStore(filename string) (*FileStore, error) {
	memoryStore := NewMemoryStore()
	store := &FileStore{
		MemoryStore: memoryStore,
		filename:    filename,
	}
	
	// Load existing data if file exists
	if _, err := os.Stat(filename); err == nil {
		if err := store.loadFromFile(); err != nil {
			// If file exists but is empty/corrupted, start fresh
			// Log the error but continue with empty store
			fmt.Printf("Warning: failed to load existing data: %v\n", err)
		}
	}
	
	return store, nil
}

func LoadFromFile(filename string) (*FileStore, error) {
	return NewFileStore(filename)
}

func (s *FileStore) loadFromFile() error {
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	
	// Handle empty file
	if len(data) == 0 {
		return nil
	}
	
	var storeData fileStoreData
	if err := json.Unmarshal(data, &storeData); err != nil {
		return fmt.Errorf("unmarshal JSON: %w", err)
	}
	
	for _, link := range storeData.Links {
		if err := s.Create(link); err != nil {
			// Skip duplicates on load
			continue
		}
	}
	
	return nil
}

func (s *FileStore) saveToFile() error {
	links, err := s.GetAll()
	if err != nil {
		return fmt.Errorf("get all links: %w", err)
	}
	
	storeData := fileStoreData{Links: links}
	data, err := json.MarshalIndent(storeData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	
	// Write to temp file first for atomicity
	tempFile := s.filename + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	
	// Atomically rename temp file to actual file
	if err := os.Rename(tempFile, s.filename); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	
	return nil
}

func (s *FileStore) Create(link *Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if err := s.MemoryStore.Create(link); err != nil {
		return err
	}
	
	// Save synchronously as per spec
	return s.saveToFile()
}

func (s *FileStore) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if err := s.MemoryStore.Delete(code); err != nil {
		return err
	}
	
	// Save synchronously as per spec
	return s.saveToFile()
}

func (s *FileStore) IncrementVisits(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if err := s.MemoryStore.IncrementVisits(code); err != nil {
		return err
	}
	
	// Save synchronously as per spec
	return s.saveToFile()
}

func (s *FileStore) Close() error {
	return s.MemoryStore.Close()
}