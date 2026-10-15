package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"shortener/handlers"
	"shortener/store"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set and non-empty")
	}

	// Create store
	st, err := store.NewStore(*dataFile)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}

	// Create handler
	apiHandler := handlers.NewAPIHandler(st, adminToken)

	// Add middleware
	handler := handlers.LoggingMiddleware(apiHandler)

	fmt.Printf("Starting URL shortener on %s with data file %s\n", *addr, *dataFile)
	log.Fatal(http.ListenAndServe(*addr, handler))
}