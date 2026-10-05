package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxBodySize = 1 * 1024 * 1024 // 1 MiB

type Handler struct {
	store      Store
	adminToken string
	mu         sync.RWMutex
}

func NewHandler(store Store, adminToken string) http.Handler {
	return &Handler{
		store:      store,
		adminToken: adminToken,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/links":
		h.handleCreateLink(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/links":
		h.handleListLinks(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/links/"):
		h.handleDeleteLink(w, r)
	case r.Method == http.MethodGet && r.URL.Path != "/" && r.URL.Path != "/api/links":
		h.handleRedirect(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.URL == "" {
		h.writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	link, err := h.store.Create(req.URL, req.Alias)
	if err != nil {
		switch err.Error() {
		case "alias already taken":
			h.writeError(w, http.StatusConflict, err.Error())
		case "invalid URL", "URL must be at most 2048 characters", "URL must start with http or https", "URL must have a host",
			"alias cannot be 'api'", "alias must be between 3 and 32 characters", "alias can only contain letters, numbers, underscores and hyphens":
			h.writeError(w, http.StatusBadRequest, err.Error())
		default:
			h.writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/links/%s", link.Code))
	w.WriteHeader(http.StatusCreated)

	resp := struct {
		Code      string    `json:"code"`
		URL       string    `json:"url"`
		CreatedAt time.Time `json:"created_at"`
		Visits    int       `json:"visits"`
	}{
		Code:      link.Code,
		URL:       link.URL,
		CreatedAt: link.CreatedAt,
		Visits:    link.Visits,
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" {
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}

	link, err := h.store.Get(code)
	if err != nil {
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}

	if err := h.store.Visit(code); err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
}

func (h *Handler) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	links, err := h.store.List()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	json.NewEncoder(w).Encode(links)
}

func (h *Handler) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if r.Method != http.MethodDelete {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	if code == "" {
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}

	if err := h.store.Delete(code); err != nil {
		if err.Error() == "link not found" {
			h.writeError(w, http.StatusNotFound, "not found")
		} else {
			h.writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) isAdmin(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	return token == h.adminToken
}

func (h *Handler) writeError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}