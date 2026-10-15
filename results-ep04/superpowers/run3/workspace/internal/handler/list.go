package handler

import (
	"encoding/json"
	"net/http"
	"sort"
)

func (h *Handler) ListLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	
	links := h.store.List()
	// Sort by CreatedAt ascending
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.Before(links[j].CreatedAt)
	})
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}