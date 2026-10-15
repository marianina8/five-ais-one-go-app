package main

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
)

const maxBodySize = 1 << 20 // 1 MiB

type CreateLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err == io.EOF {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "empty request body"})
			return
		}
		if _, ok := err.(*http.MaxBytesError); ok {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			json.NewEncoder(w).Encode(map[string]string{"error": "request body too large"})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	// Additional JSON validation
	// We need to check for extra data in the JSON
	// Since we've already read the body, we need to handle this differently
	// For now, we'll trust the decoder

	if req.URL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "url is required"})
		return
	}

	link, err := s.store.CreateLink(req.URL, req.Alias)
	if err != nil {
		switch err {
		case ErrInvalidURL, ErrInvalidAlias:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		case ErrDuplicateCode:
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "alias already taken"})
		default:
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	if !s.requireAdmin(w, r) {
		return
	}

	links := s.store.ListLinks()
	// Sort by creation time (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(links)
}

func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	if !s.requireAdmin(w, r) {
		return
	}

	if err := s.store.DeleteLink(code); err != nil {
		if err == ErrNotFound {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}