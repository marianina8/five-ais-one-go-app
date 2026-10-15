package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		fmt.Fprintln(os.Stderr, "error: ADMIN_TOKEN must be set")
		os.Exit(1)
	}

	store, err := NewStore(*dataFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to initialize store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	handler := NewHandler(store, adminToken)

	fmt.Printf("Starting server on %s\n", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "error: server failed: %v\n", err)
		os.Exit(1)
	}
}

func NewHandler(store *Store, adminToken string) http.Handler {
	mux := http.NewServeMux()
	h := &handler{
		store:      store,
		adminToken: adminToken,
	}

	// API routes
	mux.HandleFunc("POST /api/links", h.createLink)
	mux.HandleFunc("GET /api/links", h.listLinks)
	mux.HandleFunc("DELETE /api/links/{code}", h.deleteLink)

	// Redirect route
	mux.HandleFunc("GET /{code}", h.followLink)

	return mux
}

type handler struct {
	store      *Store
	adminToken string
}

func (h *handler) createLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias"`
	}

	// Limit request body to 1MB
	limitedReader := http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(limitedReader).Decode(&req); err != nil {
		// Check if error is due to request body being too large
		// http.MaxBytesReader returns a *http.MaxBytesError
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, `{"error":"malformed json"}`, http.StatusBadRequest)
		}
		return
	}

	// Validate URL and alias
	if !isValidURL(req.URL) {
		http.Error(w, `{"error":"invalid url"}`, http.StatusBadRequest)
		return
	}

	if req.Alias != "" && !isValidAlias(req.Alias) {
		http.Error(w, `{"error":"invalid alias"}`, http.StatusBadRequest)
		return
	}

	if req.Alias == "api" {
		http.Error(w, `{"error":"alias 'api' is not allowed"}`, http.StatusBadRequest)
		return
	}

	link, err := h.store.CreateLink(req.URL, req.Alias)
	if err != nil {
		if err == ErrAliasTaken {
			http.Error(w, `{"error":"alias already taken"}`, http.StatusConflict)
		} else {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *handler) followLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	code := r.PathValue("code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	link, err := h.store.GetAndIncrement(code)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (h *handler) listLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if !h.isAdmin(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	links := h.store.ListLinks()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (h *handler) deleteLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if !h.isAdmin(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	code := r.PathValue("code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	if err := h.store.DeleteLink(code); err != nil {
		http.NotFound(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) isAdmin(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if len(auth) < 7 || auth[:7] != "Bearer " {
		return false
	}
	return auth[7:] == h.adminToken
}