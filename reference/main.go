// Command shortener is the reference implementation of spec/SPEC.md: a URL-shortener service
// that stores its links in a JSON file. It is never shown to a contestant; it exists to prove
// the hidden acceptance tests are correct, so it must pass every one of them.
//
//	ADMIN_TOKEN=secret shortener -addr :8080 -data data.json
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	dataPath := flag.String("data", "data.json", "JSON file the links are stored in")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		fmt.Fprintln(os.Stderr, "shortener: ADMIN_TOKEN must be set")
		os.Exit(1)
	}
	if err := run(*addr, *dataPath, adminToken); err != nil {
		log.Fatal(err)
	}
}

// run serves until SIGINT or SIGTERM, then finishes the requests in flight.
func run(addr, dataPath, adminToken string) error {
	store, err := OpenStore(dataPath)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           (&API{store: store, adminToken: adminToken}).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", addr)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
