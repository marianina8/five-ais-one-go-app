package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func adminAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeJSONError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if auth[7:] != token {
				writeJSONError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func createLinkHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit request body size
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB

		var req struct {
			URL   string `json:"url"`
			Alias string `json:"alias,omitempty"`
		}

		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&req); err != nil {
			writeJSONError(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate URL
		if err := validateURL(req.URL); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate alias if provided
		if req.Alias != "" {
			if err := validateAlias(req.Alias); err != nil {
				writeJSONError(w, err.Error(), http.StatusBadRequest)
				return
			}
			code := req.Alias
			// Check if alias already exists
			if _, exists := store.Get(code); exists {
				writeJSONError(w, "alias already taken", http.StatusConflict)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})
	}
}

func redirectHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/")
		link, exists := store.Get(code)
		if !exists {
			writeJSONError(w, "link not found", http.StatusNotFound)
			return
		}

		// Increment visit count
		store.IncrementVisits(code)

		http.Redirect(w, r, link.URL, http.StatusFound)
	}
}

func listLinksHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		links := store.GetAll()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(links)
	}
}

func deleteLinkHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/api/links/")
		if err := store.Delete(code); err != nil {
			writeJSONError(w, "link not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func validateURL(urlStr string) error {
	if len(urlStr) > 2048 {
		return fmt.Errorf("URL too long")
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}
	return nil
}

func validateAlias(alias string) error {
	if alias == "api" {
		return fmt.Errorf("alias 'api' is not allowed")
	}
	// Simple validation - would need regex for full validation
	if len(alias) < 3 || len(alias) > 32 {
		return fmt.Errorf("alias must be 3-32 characters")
	}
	// Check for valid characters
	for _, ch := range alias {
		if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return fmt.Errorf("alias can only contain letters, numbers, hyphens, and underscores")
		}
	}
	return nil
}