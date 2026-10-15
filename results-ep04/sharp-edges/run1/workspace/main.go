package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func main() {
	addr := flag.String("addr", ":8080", "Address to listen on")
	dataFile := flag.String("data", "data.json", "Data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is not set")
	}

	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", store.handleCreateLink)
	mux.HandleFunc("GET /api/links", store.authorizeAdmin(adminToken, store.handleListLinks))
	mux.HandleFunc("DELETE /api/links/{code}", store.authorizeAdmin(adminToken, store.handleDeleteLink))
	mux.HandleFunc("GET /{code}", store.handleRedirect)
	mux.HandleFunc("HEAD /{code}", store.handleRedirect) // HEAD support for redirect

	log.Printf("Starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func isValidURL(url string) bool {
	// Basic check for scheme - more thorough validation is in store.go
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

func isValidAlias(alias string) bool {
	if alias == "api" {
		return false
	}
	match, _ := regexp.MatchString(`^[A-Za-z0-9_-]{3,32}$`, alias)
	return match
}