package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type CreateLinkRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
}

type CreateLinkResponse struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

func (h *Handler) CreateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	// Limit request body to 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	
	var req CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	
	// Validate URL
	if !validateURL(req.URL) {
		writeError(w, http.StatusBadRequest, "invalid URL")
		return
	}
	
	// Validate alias if provided
	if req.Alias != "" && !validateAlias(req.Alias) {
		writeError(w, http.StatusBadRequest, "invalid alias")
		return
	}
	
	link, err := h.store.Create(req.URL, req.Alias)
	if err != nil {
		if strings.Contains(err.Error(), "already taken") {
			writeError(w, http.StatusConflict, "alias already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	
	// Save to persistent storage
	if err := h.store.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save")
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateLinkResponse{
		Code:      link.Code,
		URL:       link.URL,
		CreatedAt: link.CreatedAt,
		Visits:    link.Visits,
	})
}