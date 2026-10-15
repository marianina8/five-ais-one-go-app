package handler

import (
	"net/http"
	"strings"
)

func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/")
	if code == "" {
		// Root path, not a redirect
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	
	link, err := h.store.Get(code)
	if err != nil {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	
	// Increment visits before redirect
	if err := h.store.IncrementVisits(code); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	
	// Save visit count
	if err := h.store.Save(); err != nil {
		// Log but don't fail redirect
		// Visit counted in memory, will be saved on next successful save
	}
	
	http.Redirect(w, r, link.URL, http.StatusFound)
}