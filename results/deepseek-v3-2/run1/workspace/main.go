package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	store, err := NewJSONStore(*dataFile)
	if err != nil {
		log.Fatal(err)
	}

	handler := NewHandler(store, adminToken)

	log.Printf("Starting server on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}

// Link represents a shortened link
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int       `json:"visits"`
}

// Store interface defines the storage operations
type Store interface {
	Create(url, alias string) (*Link, error)
	Get(code string) (*Link, error)
	IncrementVisits(code string) error
	List() ([]Link, error)
	Delete(code string) error
}

// Handler handles HTTP requests
type Handler struct {
	store      Store
	adminToken string
}

// NewHandler creates a new HTTP handler
func NewHandler(store Store, adminToken string) *Handler {
	return &Handler{
		store:      store,
		adminToken: adminToken,
	}
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/links":
		h.createLink(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/links":
		h.listLinks(w, r)
	case r.Method == http.MethodDelete && len(r.URL.Path) > len("/api/links/") && r.URL.Path[:len("/api/links/")] == "/api/links/":
		h.deleteLink(w, r)
	case r.Method == http.MethodGet && r.URL.Path != "/" && r.URL.Path != "/api/links":
		h.redirectLink(w, r)
	default:
		h.writeError(w, "Not found", http.StatusNotFound)
	}
}

// writeJSON writes JSON response
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes error response
func (h *Handler) writeError(w http.ResponseWriter, message string, status int) {
	h.writeJSON(w, status, map[string]string{"error": message})
}

// authenticate checks admin token
func (h *Handler) authenticate(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
		return false
	}
	token := authHeader[7:]
	return token == h.adminToken
}

// createLink handles POST /api/links
func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	
	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	link, err := h.store.Create(req.URL, req.Alias)
	if err != nil {
		// Map store errors to appropriate HTTP status codes
		switch err.Error() {
		case "invalid url":
			h.writeError(w, "Invalid URL", http.StatusBadRequest)
		case "invalid alias":
			h.writeError(w, "Invalid alias", http.StatusBadRequest)
		case "alias taken":
			h.writeError(w, "Alias already taken", http.StatusConflict)
		default:
			h.writeError(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	
	h.writeJSON(w, http.StatusCreated, link)
}

// listLinks handles GET /api/links
func (h *Handler) listLinks(w http.ResponseWriter, r *http.Request) {
	if !h.authenticate(r) {
		h.writeError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	links, err := h.store.List()
	if err != nil {
		h.writeError(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	h.writeJSON(w, http.StatusOK, links)
}

// deleteLink handles DELETE /api/links/{code}
func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request) {
	if !h.authenticate(r) {
		h.writeError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	code := r.URL.Path[len("/api/links/"):]
	
	if err := h.store.Delete(code); err != nil {
		if err.Error() == "not found" {
			h.writeError(w, "Not found", http.StatusNotFound)
		} else {
			h.writeError(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// redirectLink handles GET /{code}
func (h *Handler) redirectLink(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Path[1:] // Remove leading slash
	
	link, err := h.store.Get(code)
	if err != nil {
		if err.Error() == "not found" {
			h.writeError(w, "Not found", http.StatusNotFound)
		} else {
			h.writeError(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	
	// Increment visits asynchronously
	go func() {
		if err := h.store.IncrementVisits(code); err != nil {
			log.Printf("Failed to increment visits for %s: %v", code, err)
		}
	}()
	
	http.Redirect(w, r, link.URL, http.StatusFound)
}