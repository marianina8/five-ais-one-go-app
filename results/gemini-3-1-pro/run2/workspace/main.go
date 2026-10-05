package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataPath := flag.String("data", "data.json", "path to data file")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		fmt.Fprintln(os.Stderr, "ADMIN_TOKEN environment variable is required")
		os.Exit(1)
	}

	store, err := NewStore(*dataPath)
	if err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	server := &Server{
		store:      store,
		adminToken: adminToken,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", server.handleCreateLink)
	mux.HandleFunc("GET /api/links", server.handleListLinks)
	mux.HandleFunc("DELETE /api/links/{code}", server.handleDeleteLink)
	mux.HandleFunc("GET /{code}", server.handleFollowLink)

	log.Printf("Listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, wrapInterceptor(mux)))
}
