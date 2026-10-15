package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN is empty or unset")
	}

	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	server := NewServer(store, adminToken)
	log.Printf("starting server on %s", *addr)
	if err := server.Start(*addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}