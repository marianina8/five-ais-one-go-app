package handlers

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"shortener/store"
	"strings"
	"time"
)

var (
	aliasRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
	codeChars  = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	codeLength = 7
)

type APIHandler struct {
	store      *store.Store
	adminToken string
}

func NewAPIHandler(store *store.Store, adminToken string) *APIHandler {
	rand.Seed(time.Now().UnixNano())
	return &APIHandler{
		store:      store,
		adminToken: adminToken,
	}
}

func (h *APIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	
	switch {
	case path == "/api/links":
		switch r.Method {
		case "POST":
			h.handleCreateLink(w, r)
		case "GET":
			h.handleListLinks(w, r)
		default:
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	case strings.HasPrefix(path, "/api/links/"):
		h.handleLinkOperation(w, r)
	case len(path) > 1 && path[0] == '/' && path != "/api" && !strings.HasPrefix(path, "/api/"):
		h.handleRedirect(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *APIHandler) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	
	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}
	
	// Validate URL
	if req.URL == "" {
		http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
		return
	}
	
	if len(req.URL) > 2048 {
		http.Error(w, `{"error":"url too long"}`, http.StatusBadRequest)
		return
	}
	
	parsedURL, err := url.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		http.Error(w, `{"error":"Invalid URL"}`, http.StatusBadRequest)
		return
	}
	
	// Determine code
	code := req.Alias
	if code == "" {
		// Generate random code
		for i := 0; i < 100; i++ { // Try up to 100 times
			code = h.generateRandomCode()
			if !h.store.CodeExists(code) {
				break
			}
			code = ""
		}
		if code == "" {
			http.Error(w, `{"error":"Failed to generate unique code"}`, http.StatusInternalServerError)
			return
		}
	} else {
		// Validate alias
		if code == "api" {
			http.Error(w, `{"error":"alias 'api' is not allowed"}`, http.StatusBadRequest)
			return
		}
		
		if !aliasRegex.MatchString(code) {
			http.Error(w, `{"error":"alias must match ^[A-Za-z0-9_-]{3,32}$"}`, http.StatusBadRequest)
			return
		}
		
		// Check if alias already exists
		if h.store.CodeExists(code) {
			http.Error(w, `{"error":"alias already taken"}`, http.StatusConflict)
			return
		}
	}
	
	// Create link
	link, err := h.store.CreateLink(code, req.URL)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, `{"error":"alias already taken"}`, http.StatusConflict)
		} else {
			http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		}
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *APIHandler) handleRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	
	code := strings.TrimPrefix(r.URL.Path, "/")
	link, err := h.store.GetLink(code)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	
	// Increment visits in background goroutine to not block redirect
	go h.store.IncrementVisits(code)
	
	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (h *APIHandler) handleListLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	
	if !h.authenticateAdmin(r) {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	
	links := h.store.ListLinks()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (h *APIHandler) handleLinkOperation(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	
	if !h.authenticateAdmin(r) {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	
	switch r.Method {
	case "DELETE":
		if err := h.store.DeleteLink(code); err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.NotFound(w, r)
			} else {
				http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
			}
			return
		}
		
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) generateRandomCode() string {
	b := make([]rune, codeLength)
	for i := range b {
		b[i] = codeChars[rand.Intn(len(codeChars))]
	}
	return string(b)
}

func (h *APIHandler) authenticateAdmin(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}
	
	token := strings.TrimPrefix(authHeader, "Bearer ")
	return token == h.adminToken
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create response writer wrapper to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		next.ServeHTTP(rw, r)
		
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.statusCode, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}