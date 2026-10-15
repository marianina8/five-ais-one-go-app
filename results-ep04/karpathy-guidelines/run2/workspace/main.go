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
	addrFlag := flag.String("addr", ":8080", "HTTP listen address")
	dataFlag := flag.String("data", "data.json", "JSON data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	store, err := NewJSONStore(*dataFlag)
	if err != nil {
		log.Fatal(err)
	}

	handler := NewHandler(store, adminToken)

	log.Printf("Starting server on %s", *addrFlag)
	if err := http.ListenAndServe(*addrFlag, handler); err != nil {
		log.Fatal(err)
	}
}

type Link struct {
	Code      string `json:"code"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	Visits    int    `json:"visits"`
}

type Store interface {
	Create(url, alias string) (*Link, error)
	Get(code string) (*Link, error)
	Visit(code string) (*Link, error)
	List() ([]Link, error)
	Delete(code string) error
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
	}
}

func errorResponse(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

type Handler struct {
	store      Store
	adminToken string
}

func NewHandler(store Store, adminToken string) *Handler {
	return &Handler{store: store, adminToken: adminToken}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case r.Method == "POST" && path == "/api/links":
		h.createLink(w, r)
	case r.Method == "GET" && path == "/api/links":
		h.listLinks(w, r)
	case strings.HasPrefix(path, "/api/links/"):
		code := strings.TrimPrefix(path, "/api/links/")
		if r.Method == "DELETE" {
			h.deleteLink(w, r, code)
		} else {
			errorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case len(path) > 1 && path[0] == '/' && path != "/api/links" && !strings.HasPrefix(path, "/api/"):
		code := strings.TrimPrefix(path, "/")
		h.redirectLink(w, r, code)
	default:
		errorResponse(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		if err == io.EOF {
			errorResponse(w, http.StatusBadRequest, "missing request body")
		} else if strings.Contains(err.Error(), "unknown field") {
			errorResponse(w, http.StatusBadRequest, "unknown field in JSON")
		} else if strings.Contains(err.Error(), "request body too large") {
			errorResponse(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			errorResponse(w, http.StatusBadRequest, "invalid JSON")
		}
		return
	}

	link, err := h.store.Create(req.URL, req.Alias)
	if err != nil {
		switch err.Error() {
		case "URL too long":
			errorResponse(w, http.StatusBadRequest, "URL too long")
		case "URL must start with http:// or https://":
			errorResponse(w, http.StatusBadRequest, "URL must start with http:// or https://")
		case "invalid URL":
			errorResponse(w, http.StatusBadRequest, "invalid URL")
		case "alias 'api' is reserved":
			errorResponse(w, http.StatusBadRequest, "alias 'api' is reserved")
		case "alias must match ^[A-Za-z0-9_-]{3,32}$":
			errorResponse(w, http.StatusBadRequest, "alias must match ^[A-Za-z0-9_-]{3,32}$")
		case "alias already taken":
			errorResponse(w, http.StatusConflict, "alias already taken")
		case "failed to generate unique code":
			errorResponse(w, http.StatusInternalServerError, "failed to generate unique code")
		default:
			if strings.Contains(err.Error(), "failed to save") {
				errorResponse(w, http.StatusInternalServerError, "internal server error")
			} else {
				errorResponse(w, http.StatusBadRequest, err.Error())
			}
		}
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/links/%s", link.Code))
	jsonResponse(w, http.StatusCreated, link)
}

func (h *Handler) listLinks(w http.ResponseWriter, r *http.Request) {
	// Check admin authentication
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		errorResponse(w, http.StatusUnauthorized, "missing or invalid authorization")
		return
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != h.adminToken {
		errorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	links, err := h.store.List()
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	jsonResponse(w, http.StatusOK, links)
}

func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request, code string) {
	// Check admin authentication
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		errorResponse(w, http.StatusUnauthorized, "missing or invalid authorization")
		return
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != h.adminToken {
		errorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.store.Delete(code); err != nil {
		if err.Error() == "not found" {
			errorResponse(w, http.StatusNotFound, "not found")
		} else {
			errorResponse(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) redirectLink(w http.ResponseWriter, r *http.Request, code string) {
	link, err := h.store.Visit(code)
	if err != nil {
		if err.Error() == "not found" {
			errorResponse(w, http.StatusNotFound, "not found")
		} else {
			errorResponse(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}