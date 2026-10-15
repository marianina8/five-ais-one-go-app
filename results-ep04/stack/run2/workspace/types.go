package main

import (
	"time"
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type CreateRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

type CreateResponse struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}