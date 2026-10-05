package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "Address to listen on")
	dataPath := flag.String("data", "data.json", "Path to data file")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		fmt.Fprintln(os.Stderr, "ADMIN_TOKEN environment variable is required")
		os.Exit(1)
	}

	store, err := NewStore(*dataPath)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	router := NewRouter(store, adminToken)

	log.Printf("Listening on %s...", *addr)
	if err := http.ListenAndServe(*addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
