package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	
	if err := decoder.Decode(&req); err != nil {
		if err == io.EOF {
			writeJSONError(w, "missing request body", http.StatusBadRequest)
		} else if err.Error() == "http: request body too large" {
			writeJSONError(w, "request body too large", http.StatusRequestEntityTooLarge)
		} else {
			writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		}
		return
	}

	if req.URL == "" {
		writeJSONError(w, "url is required", http.StatusBadRequest)
		return
	}

	link, err := s.store.Create(req.URL, req.Alias)
	if err != nil {
		switch err.Error() {
		case "invalid URL", "URL must be http or https", "URL must have a host", "URL too long":
			writeJSONError(w, err.Error(), http.StatusBadRequest)
		case "alias cannot be 'api'", "alias must match ^[A-Za-z0-9_-]{3,32}$":
			writeJSONError(w, err.Error(), http.StatusBadRequest)
		case "code already exists":
			writeJSONError(w, err.Error(), http.StatusConflict)
		default:
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Path[1:] // Remove leading slash

	link, err := s.store.Get(code)
	if err != nil {
		writeJSONError(w, "not found", http.StatusNotFound)
		return
	}

	// Increment visits in background go routine
	go func() {
		if err := s.store.IncrementVisits(code); err != nil {
			log.Printf("failed to increment visits for %s: %v", code, err)
		}
	}()

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminRequest(r) {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	links, err := s.store.List()
	if err != nil {
		writeJSONError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if !s.isAdminRequest(r) {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	if code == "" {
		writeJSONError(w, "not found", http.StatusNotFound)
		return
	}

	if err := s.store.Delete(code); err != nil {
		writeJSONError(w, "not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) isAdminRequest(r *http.Request) bool {
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