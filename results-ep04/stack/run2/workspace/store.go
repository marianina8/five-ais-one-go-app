package main

import (
	"fmt"
	"sync"
)

type Store interface {
	Create(link *Link) error
	Get(code string) (*Link, error)
	GetAll() ([]*Link, error)
	Delete(code string) error
	IncrementVisits(code string) error
	Close() error
}

type MemoryStore struct {
	data *sync.Map
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: &sync.Map{},
	}
}

func (s *MemoryStore) Create(link *Link) error {
	_, loaded := s.data.LoadOrStore(link.Code, link)
	if loaded {
		return ErrAliasTaken
	}
	return nil
}

func (s *MemoryStore) Get(code string) (*Link, error) {
	value, ok := s.data.Load(code)
	if !ok {
		return nil, fmt.Errorf("link not found")
	}
	return value.(*Link), nil
}

func (s *MemoryStore) GetAll() ([]*Link, error) {
	var links []*Link
	s.data.Range(func(key, value interface{}) bool {
		links = append(links, value.(*Link))
		return true
	})
	return links, nil
}

func (s *MemoryStore) Delete(code string) error {
	s.data.Delete(code)
	return nil
}

func (s *MemoryStore) IncrementVisits(code string) error {
	// Use atomic operations to avoid race conditions
	for {
		value, ok := s.data.Load(code)
		if !ok {
			return fmt.Errorf("link not found")
		}
		
		oldLink := value.(*Link)
		newLink := &Link{
			Code:      oldLink.Code,
			URL:       oldLink.URL,
			CreatedAt: oldLink.CreatedAt,
			Visits:    oldLink.Visits + 1,
		}
		
		// Try to atomically swap the old link with the new one
		if swapped := s.data.CompareAndSwap(code, oldLink, newLink); swapped {
			return nil
		}
		// If CompareAndSwap failed, someone else modified it, retry
	}
}

func (s *MemoryStore) Close() error {
	return nil
}