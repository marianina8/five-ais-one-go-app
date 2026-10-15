package main

import (
	"flag"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "Address to listen on")
	dataFile := flag.String("data", "data.json", "Data file for persistence")
	flag.Parse()

	// Check for ADMIN_TOKEN
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	// Initialize store
	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	// Setup server
	server := NewServer(store, adminToken)

	log.Printf("Starting server on %s", *addr)
	if err := http.ListenAndServe(*addr, server); err != nil {
		log.Fatal(err)
	}
}