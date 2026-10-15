package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "JSON file for storing links")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	handler := NewHandler(store, adminToken)

	log.Printf("starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
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
	// Set JSON content type for all responses except redirects
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/links":
		h.createLink(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/links":
		h.listLinks(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/links/"):
		code := strings.TrimPrefix(r.URL.Path, "/api/links/")
		h.deleteLink(w, r, code)
	case r.Method == http.MethodGet && len(r.URL.Path) > 1 && r.URL.Path[0] == '/':
		code := strings.TrimPrefix(r.URL.Path, "/")
		h.followLink(w, r, code)
	default:
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSONError(w, "not found", http.StatusNotFound)
	}
}

func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	defer r.Body.Close()

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		if err == io.EOF {
			writeJSONError(w, "empty request body", http.StatusBadRequest)
			return
		}
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		writeJSONError(w, "url field is required", http.StatusBadRequest)
		return
	}

	link, err := h.store.CreateLink(req.URL, req.Alias)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "already exists"):
			writeJSONError(w, err.Error(), http.StatusConflict)
		case strings.Contains(err.Error(), "invalid URL") ||
			strings.Contains(err.Error(), "URL must be") ||
			strings.Contains(err.Error(), "alias must") ||
			strings.Contains(err.Error(), "alias 'api' is reserved") ||
			strings.Contains(err.Error(), "URL too long") ||
			strings.Contains(err.Error(), "failed to generate"):
			writeJSONError(w, err.Error(), http.StatusBadRequest)
		default:
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/links/%s", link.Code))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *Handler) listLinks(w http.ResponseWriter, r *http.Request) {
	if !h.checkAdminAuth(r) {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	links := h.store.ListLinks()
	json.NewEncoder(w).Encode(links)
}

func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request, code string) {
	if !h.checkAdminAuth(r) {
		writeJSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.store.DeleteLink(code); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, "link not found", http.StatusNotFound)
		} else {
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) followLink(w http.ResponseWriter, r *http.Request, code string) {
	link, err := h.store.GetLink(code)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, "link not found", http.StatusNotFound)
		} else {
			writeJSONError(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	if err := h.store.IncrementVisits(code); err != nil {
		// Log but continue with redirect
		log.Printf("failed to increment visits for %s: %v", code, err)
	}

	w.Header().Set("Location", link.URL)
	w.WriteHeader(http.StatusFound)
}

func (h *Handler) checkAdminAuth(r *http.Request) bool {
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

func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}