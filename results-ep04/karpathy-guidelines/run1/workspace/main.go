package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server := NewServer(store, adminToken)

	log.Printf("Starting server on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, server))
}

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

type Store interface {
	Create(url, alias string) (*Link, error)
	Get(code string) (*Link, error)
	IncrementVisits(code string) error
	List() ([]*Link, error)
	Delete(code string) error
	Close() error
}

type Server struct {
	store      Store
	adminToken string
}

func NewServer(store Store, adminToken string) *Server {
	return &Server{
		store:      store,
		adminToken: adminToken,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/links":
		s.handleListLinks(w, r)
	case r.Method == "POST" && r.URL.Path == "/api/links":
		s.handleCreateLink(w, r)
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/links/"):
		s.handleDeleteLink(w, r)
	case r.Method == "GET" && len(r.URL.Path) > 1 && r.URL.Path[0] == '/' && r.URL.Path != "/api":
		s.handleRedirect(w, r)
	default:
		writeJSONError(w, "not found", http.StatusNotFound)
	}
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}