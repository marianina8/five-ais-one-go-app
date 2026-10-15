package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	// Check ADMIN_TOKEN
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	// Create or load store
	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create server
	server := NewServer(store, adminToken)

	// Start server
	srv := &http.Server{
		Addr:    *addr,
		Handler: server,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen and serve: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
}