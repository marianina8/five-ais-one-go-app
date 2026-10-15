package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"shortener/internal/store"
)

type Handler struct {
	store      store.Store
	adminToken string
}

func NewHandler(store store.Store, adminToken string) *Handler {
	return &Handler{
		store:      store,
		adminToken: adminToken,
	}
}

func validateURL(urlStr string) bool {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != "" &&
		len(urlStr) <= 2048
}

func validateAlias(alias string) bool {
	if alias == "" {
		return true
	}
	if alias == "api" {
		return false
	}
	if len(alias) < 3 || len(alias) > 32 {
		return false
	}
	matched, _ := regexp.MatchString(`^[A-Za-z0-9_-]+$`, alias)
	return matched
}

func adminAuth(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or invalid authorization")
			return
		}
		
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != adminToken {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}