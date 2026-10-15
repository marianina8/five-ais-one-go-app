package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	addrFlag = flag.String("addr", ":8080", "HTTP address to listen on")
	dataFlag = flag.String("data", "data.json", "Path to data file")
)

func main() {
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	store, err := NewJSONStore(*dataFlag)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer store.Close()

	handler := newHandler(store, adminToken)

	log.Printf("Starting server on %s", *addrFlag)
	log.Fatal(http.ListenAndServe(*addrFlag, handler))
}

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

type Store interface {
	Create(url, alias string) (*Link, error)
	Get(code string) (*Link, error)
	IncrementVisits(code string) error
	List() ([]Link, error)
	Delete(code string) error
	Close() error
}

type handler struct {
	store      Store
	adminToken string
}

func newHandler(store Store, adminToken string) *handler {
	return &handler{
		store:      store,
		adminToken: adminToken,
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case r.Method == "GET" && path == "/api/links":
		h.authAdmin(w, r, h.listLinks)
	case r.Method == "POST" && path == "/api/links":
		h.createLink(w, r)
	case r.Method == "DELETE" && strings.HasPrefix(path, "/api/links/"):
		code := strings.TrimPrefix(path, "/api/links/")
		h.authAdmin(w, r, func(w http.ResponseWriter, r *http.Request) {
			h.deleteLink(w, r, code)
		})
	case r.Method == "GET" && len(path) > 1 && !strings.HasPrefix(path, "/api"):
		code := strings.TrimPrefix(path, "/")
		h.redirectLink(w, r, code)
	default:
		http.Error(w, `{"error": "Not found"}`, http.StatusNotFound)
	}
}

func (h *handler) authAdmin(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != h.adminToken {
		writeJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	next(w, r)
}

func (h *handler) listLinks(w http.ResponseWriter, r *http.Request) {
	links, err := h.store.List()
	if err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (h *handler) createLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	link, err := h.store.Create(req.URL, req.Alias)
	if err != nil {
		switch err {
		case ErrInvalidURL:
			writeJSONError(w, "Invalid URL", http.StatusBadRequest)
		case ErrInvalidAlias:
			writeJSONError(w, "Invalid alias", http.StatusBadRequest)
		case ErrAliasTaken:
			writeJSONError(w, "Alias already taken", http.StatusConflict)
		default:
			writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *handler) redirectLink(w http.ResponseWriter, r *http.Request, code string) {
	link, err := h.store.Get(code)
	if err != nil {
		writeJSONError(w, "Link not found", http.StatusNotFound)
		return
	}

	if err := h.store.IncrementVisits(code); err != nil {
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (h *handler) deleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if err := h.store.Delete(code); err != nil {
		if err == ErrNotFound {
			writeJSONError(w, "Link not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

var (
	ErrInvalidURL   = fmt.Errorf("invalid URL")
	ErrInvalidAlias = fmt.Errorf("invalid alias")
	ErrAliasTaken   = fmt.Errorf("alias already taken")
	ErrNotFound     = fmt.Errorf("not found")
)