package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP server address")
	dataPath := flag.String("data", "data.json", "Path to data file")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	store, err := NewFileStore(*dataPath)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", createLinkHandler(store))
	mux.Handle("GET /api/links", adminAuth(adminToken)(http.HandlerFunc(listLinksHandler(store))))
	mux.HandleFunc("DELETE /api/links/{code}", deleteLinkHandler(store))
	mux.HandleFunc("GET /{code}", redirectHandler(store))

	fmt.Printf("Starting server on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}