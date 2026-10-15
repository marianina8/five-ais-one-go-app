package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type CreateLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

type CreateLinkResponse struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type Handler struct {
	store      *Store
	adminToken string
}

func NewHandler(store *Store, adminToken string) *Handler {
	return &Handler{
		store:      store,
		adminToken: adminToken,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/api/links" && r.Method == http.MethodPost:
		h.handleCreateLink(w, r)
	case path == "/api/links" && r.Method == http.MethodGet:
		h.handleListLinks(w, r)
	case strings.HasPrefix(path, "/api/links/") && r.Method == http.MethodDelete:
		code := strings.TrimPrefix(path, "/api/links/")
		h.handleDeleteLink(w, r, code)
	case len(path) > 1 && path != "/api" && path != "/api/":
		code := strings.TrimPrefix(path, "/")
		h.handleRedirect(w, r, code)
	default:
		h.writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()

	var req CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate request
	if errors, ok := validateCreateLinkRequest(req); !ok {
		h.writeError(w, http.StatusBadRequest, strings.Join(errors, "; "))
		return
	}

	link, err := h.store.CreateLink(req.URL, req.Alias)
	if err != nil {
		if err.Error() == "alias already taken" {
			h.writeError(w, http.StatusConflict, "alias already taken")
		} else {
			h.writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	resp := CreateLinkResponse{
		Code:      link.Code,
		URL:       link.URL,
		CreatedAt: link.CreatedAt,
		Visits:    link.Visits,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleRedirect(w http.ResponseWriter, r *http.Request, code string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	link, ok := h.store.GetLink(code)
	if !ok {
		h.writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Increment visit count in background
	go func() {
		if err := h.store.IncrementVisits(link.Code); err != nil {
			fmt.Printf("Failed to increment visits for %s: %v\n", link.Code, err)
		}
	}()

	// Handle HEAD request
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (h *Handler) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Check admin auth
	if !h.isAdminRequest(r) {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Handle HEAD request
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	links := h.store.GetAllLinks()
	
	// Sort by created_at (oldest first)
	for i := 0; i < len(links); i++ {
		for j := i + 1; j < len(links); j++ {
			if links[j].CreatedAt.Before(links[i].CreatedAt) {
				links[i], links[j] = links[j], links[i]
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (h *Handler) handleDeleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if r.Method != http.MethodDelete {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Check admin auth
	if !h.isAdminRequest(r) {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Validate code is not empty
	if code == "" {
		h.writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	if err := h.store.DeleteLink(code); err != nil {
		if err.Error() == "link not found" {
			h.writeError(w, http.StatusNotFound, "not found")
		} else {
			h.writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) isAdminRequest(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return false
	}

	return parts[1] == h.adminToken
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}