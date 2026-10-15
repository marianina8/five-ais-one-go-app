package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	codeLen = 7
	maxBodySize = 1 << 20 // 1 MiB
)

func generateRandomCode() (string, error) {
	b := make([]byte, codeLen)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("generate random: %w", err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

func handleCreateLink(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit request body size
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		
		// Validate URL
		if err := ValidateURL(req.URL); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid URL")
			return
		}
		
		// Validate alias if provided
		if err := ValidateAlias(req.Alias); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid alias")
			return
		}
		
		// Generate or use provided code
		code := req.Alias
		if code == "" {
			// Generate unique code
			for i := 0; i < 10; i++ { // Try up to 10 times
				generated, err := generateRandomCode()
				if err != nil {
					writeJSONError(w, http.StatusInternalServerError, "failed to generate code")
					return
				}
				
				// Check if code exists
				if _, err := store.Get(generated); err != nil {
					code = generated
					break
				}
			}
			if code == "" {
				writeJSONError(w, http.StatusInternalServerError, "failed to generate unique code")
				return
			}
		}
		
		// Create link
		link := &Link{
			Code:      code,
			URL:       req.URL,
			CreatedAt: time.Now(),
			Visits:    0,
		}
		
		if err := store.Create(link); err != nil {
			if err == ErrAliasTaken {
				writeJSONError(w, http.StatusConflict, "alias already taken")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "failed to create link")
			}
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateResponse{
			Code:      link.Code,
			URL:       link.URL,
			CreatedAt: link.CreatedAt,
			Visits:    link.Visits,
		})
	}
}

func handleGetLink(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/")
		
		link, err := store.Get(code)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, "link not found")
			return
		}
		
		// Increment visits
		store.IncrementVisits(code)
		
		http.Redirect(w, r, link.URL, http.StatusFound)
	}
}

func adminAuthMiddleware(adminToken string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing authorization")
			return
		}
		
		prefix := "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			writeJSONError(w, http.StatusUnauthorized, "invalid authorization format")
			return
		}
		
		token := strings.TrimPrefix(authHeader, prefix)
		if token != adminToken {
			writeJSONError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		
		next(w, r)
	}
}

func handleListLinks(store Store, adminToken string) http.HandlerFunc {
	return adminAuthMiddleware(adminToken, func(w http.ResponseWriter, r *http.Request) {
		links, err := store.GetAll()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list links")
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(links)
	})
}

func handleDeleteLink(store Store, adminToken string) http.HandlerFunc {
	return adminAuthMiddleware(adminToken, func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/api/links/")
		
		if err := store.Delete(code); err != nil {
			writeJSONError(w, http.StatusNotFound, "link not found")
			return
		}
		
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}