package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Parse command-line flags
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	// Check ADMIN_TOKEN environment variable
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	// Initialize store
	store, err := newFileStore(*dataFile)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}
	defer store.Close()

	// Create handler
	handler := newHandler(store, adminToken)

	// Create HTTP server
	srv := &http.Server{
		Addr:         *addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Starting server on %s", *addr)
	log.Printf("Using data file: %s", *dataFile)
	
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}