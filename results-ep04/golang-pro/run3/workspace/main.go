package main

import (
	"flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

func main() {
	// Seed random number generator
	rand.New(rand.NewSource(time.Now().UnixNano()))

	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "path to data file")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	store, err := NewJSONStore(*dataFile)
	if err != nil {
		log.Fatal(err)
	}

	srv := &Server{
		store:      store,
		adminToken: adminToken,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{code}", srv.handleRedirect)
	mux.HandleFunc("POST /api/links", srv.handleCreateLink)
	mux.HandleFunc("GET /api/links", srv.handleListLinks)
	mux.HandleFunc("DELETE /api/links/{code}", srv.handleDeleteLink)

	log.Printf("Starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}