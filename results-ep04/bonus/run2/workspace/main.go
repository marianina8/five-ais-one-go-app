package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
)

func main() {
	var addr, dataFile string
	flag.StringVar(&addr, "addr", ":8080", "address to listen on")
	flag.StringVar(&dataFile, "data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	store, err := NewStore(dataFile)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, adminToken)

	log.Printf("Starting server on %s", addr)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// JSON error response
type ErrorResponse struct {
	Error string `json:"error"`
}

func jsonError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}