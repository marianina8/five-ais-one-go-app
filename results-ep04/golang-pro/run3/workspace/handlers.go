package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	code := r.PathValue("code")
	if code == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "code required"}`, http.StatusBadRequest)
		return
	}

	link, err := s.store.GetLink(code)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "link not found"}`, http.StatusNotFound)
		return
	}

	if err := s.store.IncrementVisits(code); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size (1 MiB)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		var maxBytesErr *http.MaxBytesError
		switch {
		case errors.As(err, &syntaxErr):
			http.Error(w, `{"error": "invalid JSON"}`, http.StatusBadRequest)
		case errors.As(err, &unmarshalErr):
			http.Error(w, `{"error": "invalid field type"}`, http.StatusBadRequest)
		case errors.As(err, &maxBytesErr):
			http.Error(w, `{"error": "request body too large"}`, http.StatusRequestEntityTooLarge)
		case err == io.EOF:
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
		default:
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
		}
		return
	}

	link, err := s.store.CreateLink(req.URL, req.Alias)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		status := http.StatusBadRequest
		if err.Error() == "alias already taken" || err.Error() == "code already exists" {
			status = http.StatusConflict
		}
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if !s.isAdmin(r) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}

	links, err := s.store.ListLinks()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if !s.isAdmin(r) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
		return
	}

	code := r.PathValue("code")
	if code == "" {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "code required"}`, http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteLink(code); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err.Error() == "link not found" {
			http.Error(w, `{"error": "link not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) isAdmin(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}

	return parts[1] == s.adminToken
}