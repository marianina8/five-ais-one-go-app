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
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	// Create store
	store, err := NewStore(*dataPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create server
	server := NewServer(*addr, store, adminToken)

	fmt.Printf("Starting server on %s with data file %s\n", *addr, *dataPath)
	
	if err := http.ListenAndServe(*addr, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}