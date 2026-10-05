package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func run() error {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	data := flag.String("data", "data.json", "JSON storage file")
	flag.Parse()
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		return fmt.Errorf("ADMIN_TOKEN must be set and non-empty")
	}
	store, err := openStore(*data)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           newHandler(store, token),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("listening on %s", *addr)
	err = srv.ListenAndServe()
	if err != http.ErrServerClosed {
		// Release the shutdown goroutine if listening failed.
		stop()
		<-finished
		return err
	}
	<-finished
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
