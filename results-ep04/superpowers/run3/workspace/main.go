package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "server address")
	dataFile := flag.String("data", "data.json", "data file path")
	flag.Parse()

	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable must be set")
	}

	fmt.Printf("Starting server on %s with data file %s\n", *addr, *dataFile)
}