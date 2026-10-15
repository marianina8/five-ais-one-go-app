package main

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// handler implements the URL shortener HTTP API
type handler struct {
	store      *FileStore
	adminToken string
}

// newHandler creates a new HTTP handler
func newHandler(store *FileStore, adminToken string) http.Handler {
	return &handler{
		store:      store,
		adminToken: adminToken,
	}
}

// ServeHTTP routes requests to the appropriate handler
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set content type for JSON responses
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == "POST" && r.URL.Path == "/api/links":
		h.handleCreateLink(w, r)
	case r.Method == "GET" && r.URL.Path == "/api/links":
		h.handleListLinks(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/links/"):
		code := strings.TrimPrefix(r.URL.Path, "/api/links/")
		if r.Method == "DELETE" {
			h.handleDeleteLink(w, r, code)
		} else {
			h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case r.Method == "GET" && r.URL.Path != "/api/links":
		code := strings.TrimPrefix(r.URL.Path, "/")
		h.handleRedirect(w, r, code)
	default:
		h.writeError(w, http.StatusNotFound, "not found")
	}
}

// CreateLinkRequest represents a request to create a new link
type CreateLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

// CreateLinkResponse represents the response when creating a link
type CreateLinkResponse struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

// handleCreateLink handles POST /api/links
func (h *handler) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Validate URL
	if err := validateURL(req.URL); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate alias if provided
	code := req.Alias
	if code == "" {
		// Generate random code
		code = generateRandomCode(7)
	} else {
		if err := validateAlias(code); err != nil {
			h.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Create link
	link := Link{
		Code:      code,
		URL:       req.URL,
		CreatedAt: time.Now(),
		Visits:    0,
	}

	if err := h.store.Create(link); err != nil {
		if storeErr, ok := err.(*StoreError); ok && storeErr.Code == "duplicate" {
			h.writeError(w, http.StatusConflict, "alias already taken")
		} else {
			h.writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	// Return response
	resp := CreateLinkResponse{
		Code:      code,
		URL:       req.URL,
		CreatedAt: link.CreatedAt,
		Visits:    0,
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// handleRedirect handles GET /{code}
func (h *handler) handleRedirect(w http.ResponseWriter, r *http.Request, code string) {
	link, err := h.store.Get(code)
	if err != nil {
		if storeErr, ok := err.(*StoreError); ok && storeErr.Code == "not_found" {
			h.writeError(w, http.StatusNotFound, "not found")
		} else {
			h.writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	// Increment visits
	if err := h.store.IncrementVisits(code); err != nil {
		// Log error but still redirect
		_ = err
	}

	// Redirect
	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
}

// handleListLinks handles GET /api/links
func (h *handler) handleListLinks(w http.ResponseWriter, r *http.Request) {
	// Check admin authentication
	if !h.checkAdminAuth(r) {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	links := h.store.List()
	
	// Convert to response format
	resp := make([]CreateLinkResponse, len(links))
	for i, link := range links {
		resp[i] = CreateLinkResponse{
			Code:      link.Code,
			URL:       link.URL,
			CreatedAt: link.CreatedAt,
			Visits:    link.Visits,
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleDeleteLink handles DELETE /api/links/{code}
func (h *handler) handleDeleteLink(w http.ResponseWriter, r *http.Request, code string) {
	// Check admin authentication
	if !h.checkAdminAuth(r) {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.store.Delete(code); err != nil {
		if storeErr, ok := err.(*StoreError); ok && storeErr.Code == "not_found" {
			h.writeError(w, http.StatusNotFound, "not found")
		} else {
			h.writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// checkAdminAuth checks if the request has valid admin authorization
func (h *handler) checkAdminAuth(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	// Check Bearer token
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}

	return parts[1] == h.adminToken
}

// writeError writes a JSON error response
func (h *handler) writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// validateURL validates a URL according to spec requirements
func validateURL(urlStr string) error {
	if len(urlStr) > 2048 {
		return &ValidationError{message: "URL too long"}
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return &ValidationError{message: "invalid URL"}
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return &ValidationError{message: "URL scheme must be http or https"}
	}

	if u.Host == "" {
		return &ValidationError{message: "URL must have a host"}
	}

	return nil
}

// validateAlias validates an alias according to spec requirements
func validateAlias(alias string) error {
	if alias == "api" {
		return &ValidationError{message: "alias 'api' is reserved"}
	}

	// ^[A-Za-z0-9_-]{3,32}$
	match, _ := regexp.MatchString(`^[A-Za-z0-9_-]{3,32}$`, alias)
	if !match {
		return &ValidationError{message: "alias must be 3-32 characters, containing only letters, numbers, underscores and hyphens"}
	}

	return nil
}

// generateRandomCode generates a random code of length n
func generateRandomCode(n int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, n)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return string(result)
}

// ValidationError represents a validation error
type ValidationError struct {
	message string
}

func (e *ValidationError) Error() string {
	return e.message
}