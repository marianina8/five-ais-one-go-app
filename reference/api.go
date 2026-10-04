package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

const maxRequestBody = 1 << 20 // 1 MiB

// API serves the HTTP endpoints from SPEC.md.
type API struct {
	store      *Store
	adminToken string
}

// Routes returns the handler for every endpoint. The method in each pattern makes
// ServeMux answer 405 for other methods on a known path (a GET pattern also matches HEAD).
func (api *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", api.createLink)
	mux.HandleFunc("GET /api/links", api.requireAdmin(api.listLinks))
	mux.HandleFunc("DELETE /api/links/{code}", api.requireAdmin(api.deleteLink))
	mux.HandleFunc("GET /{code}", api.followLink)
	return jsonErrors(mux)
}

// jsonErrors makes ServeMux's own 404 and 405 answers JSON like every other error.
func jsonErrors(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		// No route matched: let ServeMux pick 404 or 405 (and set Allow), then write JSON instead of its text.
		recorder := &statusRecorder{header: http.Header{}}
		mux.ServeHTTP(recorder, r)
		if allow := recorder.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		writeError(w, recorder.status, http.StatusText(recorder.status))
	})
}

// statusRecorder keeps the status and headers ServeMux writes and throws the body away.
type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header         { return r.header }
func (r *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *statusRecorder) WriteHeader(status int)      { r.status = status }

type createRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias"`
}

func (api *API) createLink(w http.ResponseWriter, r *http.Request) {
	req, status, err := decodeCreateRequest(w, r)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	if !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "url must be an absolute http or https URL of at most 2048 characters")
		return
	}
	if req.Alias != "" && !validAlias(req.Alias) {
		writeError(w, http.StatusBadRequest, "alias must be 3-32 letters, digits, _ or -, and not \"api\"")
		return
	}

	link, err := api.store.Create(req.URL, req.Alias)
	switch {
	case errors.Is(err, ErrAliasTaken):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		api.internalError(w, "create link", err)
	default:
		writeJSON(w, http.StatusCreated, link)
	}
}

// decodeCreateRequest reads the JSON body strictly: at most 1 MiB, one object, no unknown fields.
func decodeCreateRequest(w http.ResponseWriter, r *http.Request) (createRequest, int, error) {
	var req createRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return req, http.StatusRequestEntityTooLarge, errors.New("request body too large")
		}
		return req, http.StatusBadRequest, errors.New("invalid JSON: " + err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return req, http.StatusBadRequest, errors.New("invalid JSON: unexpected data after the object")
	}
	return req, 0, nil
}

func (api *API) followLink(w http.ResponseWriter, r *http.Request) {
	target, found, err := api.store.Visit(r.PathValue("code"))
	switch {
	case err != nil:
		api.internalError(w, "count visit", err)
	case !found:
		writeError(w, http.StatusNotFound, "link not found")
	default:
		http.Redirect(w, r, target, http.StatusFound)
	}
}

func (api *API) listLinks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.store.List())
}

func (api *API) deleteLink(w http.ResponseWriter, r *http.Request) {
	found, err := api.store.Delete(r.PathValue("code"))
	switch {
	case err != nil:
		api.internalError(w, "delete link", err)
	case !found:
		writeError(w, http.StatusNotFound, "link not found")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// requireAdmin lets a request through only with "Authorization: Bearer <admin token>".
// The comparison takes the same time whatever the token, so it can't be guessed byte by byte.
func (api *API) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, isBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !isBearer || subtle.ConstantTimeCompare([]byte(token), []byte(api.adminToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "missing or wrong admin token")
			return
		}
		next(w, r)
	}
}

// internalError logs the cause and gives the client a generic 500.
func (api *API) internalError(w http.ResponseWriter, action string, err error) {
	log.Printf("%s: %v", action, err)
	writeError(w, http.StatusInternalServerError, "could not "+action)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
