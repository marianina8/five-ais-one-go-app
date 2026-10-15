package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	maxBodySize   = 1 << 20 // 1 MiB
	maxURLLength  = 2048
	codeChars     = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	codeLength    = 7
	maxRetries    = 10
)

var (
	aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	httpRegex    = regexp.MustCompile(`^https?://`)
)

type Handler struct {
	store      *JSONStore
	adminToken string
	rand       *rand.Rand
}

type CreateRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewHandler(store *JSONStore, adminToken string) *Handler {
	return &Handler{
		store:      store,
		adminToken: adminToken,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	switch {
	case r.Method == http.MethodGet && parts[0] == "":
		h.redirectToDefault(w, r)
	case r.Method == http.MethodGet && len(parts) == 1 && parts[0] != "api":
		h.followLink(w, r, parts[0])
	case r.Method == http.MethodPost && len(parts) == 2 && parts[0] == "api" && parts[1] == "links":
		h.createLink(w, r)
	case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "api" && parts[1] == "links":
		h.listLinks(w, r)
	case r.Method == http.MethodDelete && len(parts) == 3 && parts[0] == "api" && parts[1] == "links":
		h.deleteLink(w, r, parts[2])
	default:
		h.methodNotAllowed(w)
	}
}

func (h *Handler) redirectToDefault(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/api/links", http.StatusFound)
}

func (h *Handler) followLink(w http.ResponseWriter, r *http.Request, code string) {
	link, err := h.store.Get(code)
	if err != nil {
		h.error(w, "Link not found", http.StatusNotFound)
		return
	}

	if err := h.store.IncrementVisits(code); err != nil {
		h.error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err == io.EOF {
			h.error(w, "Empty request body", http.StatusBadRequest)
		} else if strings.Contains(err.Error(), "request body too large") {
			h.error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		} else {
			h.error(w, "Invalid JSON", http.StatusBadRequest)
		}
		return
	}

	if err := h.validateCreateRequest(req); err != nil {
		h.error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var link *Link
	var err error

	if req.Alias != "" {
		// User provided an alias - try once
		link, err = h.store.Create(req.URL, req.Alias)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				h.error(w, "Alias already taken", http.StatusConflict)
			} else {
				h.error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Generate a random code, retry on conflicts
		for i := 0; i < maxRetries; i++ {
			code := h.generateCode()
			link, err = h.store.Create(req.URL, code)
			if err == nil {
				break // Success
			}
			if !strings.Contains(err.Error(), "already exists") {
				// Some other error
				h.error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			// Conflict - try again with new code
		}

		if err != nil {
			// Failed after maxRetries
			h.error(w, "Failed to generate unique code", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *Handler) listLinks(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateAdmin(r) {
		h.error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	links := h.store.List()
	
	// Sort by CreatedAt (oldest first)
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if !h.authenticateAdmin(r) {
		h.error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.store.Delete(code); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.error(w, "Link not found", http.StatusNotFound)
		} else {
			h.error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST, DELETE")
	h.error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (h *Handler) authenticateAdmin(r *http.Request) bool {
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

func (h *Handler) validateCreateRequest(req CreateRequest) error {
	// Validate URL
	if req.URL == "" {
		return fmt.Errorf("url is required")
	}

	if len(req.URL) > maxURLLength {
		return fmt.Errorf("url too long")
	}

	if !httpRegex.MatchString(req.URL) {
		return fmt.Errorf("url must be http or https")
	}

	parsed, err := url.Parse(req.URL)
	if err != nil {
		return fmt.Errorf("invalid url")
	}

	if parsed.Host == "" {
		return fmt.Errorf("url must have a host")
	}

	// Validate alias if provided
	if req.Alias != "" {
		if !aliasPattern.MatchString(req.Alias) {
			return fmt.Errorf("alias must match ^[A-Za-z0-9_-]{3,32}$")
		}

		if strings.ToLower(req.Alias) == "api" {
			return fmt.Errorf("alias cannot be 'api'")
		}
	}

	return nil
}

func (h *Handler) generateCode() string {
	code := make([]byte, codeLength)
	for i := range code {
		code[i] = codeChars[h.rand.Intn(len(codeChars))]
	}
	return string(code)
}

func (h *Handler) error(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}