package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	data := flag.String("data", "data.json", "JSON storage file")
	flag.Parse()
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: ADMIN_TOKEN must be set and non-empty")
		os.Exit(1)
	}
	store, err := OpenStore(*data)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	server := &http.Server{
		Addr:              *addr,
		Handler:           NewHandler(store, token),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("listening on %s", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
