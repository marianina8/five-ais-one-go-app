// Reference implementation of SPEC.md. Private: never shown to a contestant.
// It exists to prove the hidden acceptance tests are correct (it must pass all of them).
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	maxURLLen  = 2048
	maxBody    = 1 << 20
	codeLen    = 7
	codeChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	reservedID = "api"
)

var aliasRE = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

// Link is one shortened URL.
type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

// Store keeps links in memory and writes them to a JSON file after every change.
type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	seq   []string // codes in creation order
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]*Link{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var links []*Link
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	sort.SliceStable(links, func(i, j int) bool { return links[i].CreatedAt.Before(links[j].CreatedAt) })
	for _, l := range links {
		s.links[l.Code] = l
		s.seq = append(s.seq, l.Code)
	}
	return s, nil
}

// saveLocked writes all links atomically: temp file, fsync, rename. Caller holds s.mu.
func (s *Store) saveLocked() error {
	list := s.listLocked()
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".links-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func (s *Store) listLocked() []Link {
	out := make([]Link, 0, len(s.seq))
	for _, c := range s.seq {
		out = append(out, *s.links[c])
	}
	return out
}

var errTaken = errors.New("alias already taken")

func (s *Store) Create(rawURL, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code == "" {
		for {
			c, err := randomCode()
			if err != nil {
				return Link{}, err
			}
			if _, ok := s.links[c]; !ok && c != reservedID {
				code = c
				break
			}
		}
	} else if _, ok := s.links[code]; ok {
		return Link{}, errTaken
	}
	l := &Link{Code: code, URL: rawURL, CreatedAt: time.Now().UTC()}
	s.links[code] = l
	s.seq = append(s.seq, code)
	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		s.seq = s.seq[:len(s.seq)-1]
		return Link{}, err
	}
	return *l, nil
}

// Visit returns the target URL and counts the visit.
func (s *Store) Visit(code string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return "", false, nil
	}
	l.Visits++
	if err := s.saveLocked(); err != nil {
		l.Visits--
		return "", true, err
	}
	return l.URL, true, nil
}

func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Store) Delete(code string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return false, nil
	}
	delete(s.links, code)
	idx := -1
	for i, c := range s.seq {
		if c == code {
			idx = i
			break
		}
	}
	s.seq = append(s.seq[:idx], s.seq[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		s.links[code] = l
		s.seq = append(s.seq[:idx], append([]string{code}, s.seq[idx:]...)...)
		return false, err
	}
	return true, nil
}

func randomCode() (string, error) {
	b := make([]byte, codeLen)
	n := big.NewInt(int64(len(codeChars)))
	for i := range b {
		r, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", err
		}
		b[i] = codeChars[r.Int64()]
	}
	return string(b), nil
}

func validURL(raw string) bool {
	if raw == "" || len(raw) > maxURLLen {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Server is the HTTP API.
type Server struct {
	store *Store
	token string
}

func (srv *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", srv.create)
	mux.HandleFunc("GET /api/links", srv.admin(srv.list))
	mux.HandleFunc("DELETE /api/links/{code}", srv.admin(srv.delete))
	mux.HandleFunc("GET /{code}", srv.redirect)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (srv *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(srv.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (srv *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req struct {
		URL   string `json:"url"`
		Alias string `json:"alias"`
	}
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !validURL(req.URL) {
		writeError(w, http.StatusBadRequest, "url must be an absolute http or https URL of at most 2048 characters")
		return
	}
	if req.Alias != "" && (!aliasRE.MatchString(req.Alias) || req.Alias == reservedID) {
		writeError(w, http.StatusBadRequest, "invalid alias")
		return
	}
	link, err := srv.store.Create(req.URL, req.Alias)
	switch {
	case errors.Is(err, errTaken):
		writeError(w, http.StatusConflict, "alias already taken")
	case err != nil:
		log.Printf("create: %v", err)
		writeError(w, http.StatusInternalServerError, "could not save link")
	default:
		writeJSON(w, http.StatusCreated, link)
	}
}

func (srv *Server) redirect(w http.ResponseWriter, r *http.Request) {
	target, ok, err := srv.store.Visit(r.PathValue("code"))
	switch {
	case err != nil:
		log.Printf("visit: %v", err)
		writeError(w, http.StatusInternalServerError, "could not save visit")
	case !ok:
		writeError(w, http.StatusNotFound, "not found")
	default:
		http.Redirect(w, r, target, http.StatusFound)
	}
}

func (srv *Server) list(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, srv.store.List())
}

func (srv *Server) delete(w http.ResponseWriter, r *http.Request) {
	ok, err := srv.store.Delete(r.PathValue("code"))
	switch {
	case err != nil:
		log.Printf("delete: %v", err)
		writeError(w, http.StatusInternalServerError, "could not delete link")
	case !ok:
		writeError(w, http.StatusNotFound, "not found")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dataPath := flag.String("data", "data.json", "JSON file for links")
	flag.Parse()

	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "ADMIN_TOKEN must be set")
		os.Exit(1)
	}
	store, err := OpenStore(*dataPath)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           (&Server{store: store, token: token}).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
