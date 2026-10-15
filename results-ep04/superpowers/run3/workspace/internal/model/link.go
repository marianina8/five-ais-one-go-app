package model

import "time"

type Link struct {
	URL       string    `json:"url"`
	Code      string    `json:"code"`
	Alias     string    `json:"alias,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}