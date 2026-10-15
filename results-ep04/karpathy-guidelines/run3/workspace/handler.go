package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	maxRequestBody = 1 << 20 // 1 MiB
	maxRetries     = 10      // Maximum attempts to generate unique random code
)

var (
	aliasRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
)

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
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/links":
		h.createLink(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/links":
		h.authorizeAdmin(w, r, h.listLinks)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/links/"):
		h.authorizeAdmin(w, r, h.deleteLink)
	case r.Method == http.MethodGet && r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/api/"):
		h.redirectLink(w, r)
	default:
		h.methodNotAllowed(w, r)
	}
}

func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	defer r.Body.Close()

	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.URL == "" {
		h.jsonError(w, http.StatusBadRequest, "url is required")
		return
	}

	u, err := url.Parse(req.URL)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		h.jsonError(w, http.StatusBadRequest, "url must be a valid http or https URL")
		return
	}

	if len(req.URL) > 2048 {
		h.jsonError(w, http.StatusBadRequest, "url too long")
		return
	}

	var code string
	if req.Alias != "" {
		if !aliasRegexp.MatchString(req.Alias) || req.Alias == "api" {
			h.jsonError(w, http.StatusBadRequest, "alias must match ^[A-Za-z0-9_-]{3,32}$ and not be 'api'")
			return
		}
		code = req.Alias
	} else {
		// Generate unique random code
		for i := 0; i < maxRetries; i++ {
			code = generateRandomCode(7)
			_, err := h.store.Get(code)
			if err != nil {
				// Code doesn't exist, we can use it
				break
			}
			if i == maxRetries-1 {
				h.jsonError(w, http.StatusInternalServerError, "failed to generate unique code")
				return
			}
		}
	}

	link, err := h.store.Create(code, req.URL)
	if err != nil {
		if err.Error() == "code already exists" {
			h.jsonError(w, http.StatusConflict, "alias already taken")
			return
		}
		h.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *Handler) redirectLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/")
	link, err := h.store.Get(code)
	if err != nil {
		h.jsonError(w, http.StatusNotFound, "link not found")
		return
	}

	if err := h.store.IncrementVisits(code); err != nil {
		h.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (h *Handler) listLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	links := h.store.List()
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}

func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/api/links/")
	if code == "" {
		h.jsonError(w, http.StatusNotFound, "link not found")
		return
	}

	if err := h.store.Delete(code); err != nil {
		if err.Error() == "link not found" {
			h.jsonError(w, http.StatusNotFound, "link not found")
			return
		}
		h.jsonError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) authorizeAdmin(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		h.jsonError(w, http.StatusUnauthorized, "missing or invalid authorization")
		return
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	if token != h.adminToken {
		h.jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	next(w, r)
}

func (h *Handler) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		switch {
		case r.URL.Path == "/api/links":
			h.listLinks(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/links/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			code := strings.TrimPrefix(r.URL.Path, "/")
			if code != "" && !strings.HasPrefix(code, "api/") {
				_, err := h.store.Get(code)
				if err != nil {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.WriteHeader(http.StatusFound)
				}
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}
	http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
}

func (h *Handler) jsonError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}