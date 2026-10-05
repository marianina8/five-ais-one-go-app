package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

const maxBody = 1 << 20

type handler struct {
	store     *Store
	tokenHash [32]byte
}

func NewHandler(store *Store, token string) http.Handler {
	return &handler{store: store, tokenHash: sha256.Sum256([]byte("Bearer " + token))}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *handler) authorized(w http.ResponseWriter, r *http.Request) bool {
	got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
	if subtle.ConstantTimeCompare(got[:], h.tokenHash[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (h *handler) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("storage error: %v", err)
		writeError(w, http.StatusInternalServerError, "storage failure")
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/links":
		switch r.Method {
		case http.MethodPost:
			h.create(w, r)
		case http.MethodGet:
			if h.authorized(w, r) {
				writeJSON(w, http.StatusOK, h.store.List())
			}
		default:
			methodNotAllowed(w, "GET, POST")
		}
	case strings.HasPrefix(path, "/api/links/") && len(strings.TrimPrefix(path, "/api/links/")) > 0 && !strings.Contains(strings.TrimPrefix(path, "/api/links/"), "/"):
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, "DELETE")
			return
		}
		if !h.authorized(w, r) {
			return
		}
		if err := h.store.Delete(strings.TrimPrefix(path, "/api/links/")); err != nil {
			h.storeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(path, "/") && len(path) > 1 && !strings.Contains(path[1:], "/"):
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		link, err := h.store.Follow(path[1:])
		if err != nil {
			h.storeError(w, err)
			return
		}
		w.Header().Set("Location", link.URL)
		w.WriteHeader(http.StatusFound)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return
	}
	var request struct {
		URL   string          `json:"url"`
		Alias json.RawMessage `json:"alias"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := requireEOF(dec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validURL(request.URL) {
		writeError(w, http.StatusBadRequest, "invalid URL")
		return
	}
	var alias *string
	if request.Alias != nil {
		var value string
		if err := json.Unmarshal(request.Alias, &value); err != nil || !validAlias(value) {
			writeError(w, http.StatusBadRequest, "invalid alias")
			return
		}
		alias = &value
	}
	link, err := h.store.Create(request.URL, alias)
	if err != nil {
		h.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}
