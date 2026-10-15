package store

import "shortener/internal/model"

type Store interface {
	Create(url, alias string) (*model.Link, error)
	Get(code string) (*model.Link, error)
	IncrementVisits(code string) error
	Delete(code string) error
	List() []*model.Link
	Save() error
	Load() error
}