package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func ExampleUsage() {
	// This is an example of how to use the service programmatically
	tmpDir := os.TempDir()
	dataFile := filepath.Join(tmpDir, "example-data.json")
	defer os.RemoveAll(dataFile)

	// Set up environment
	os.Setenv("ADMIN_TOKEN", "example-token")
	
	store, err := NewStore(dataFile)
	if err != nil {
		fmt.Printf("Error creating store: %v\n", err)
		return
	}
	defer store.Close()

	handler := NewHandler(store, "example-token")

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Example 1: Create a link with alias
	fmt.Println("Creating link with alias 'example'...")
	createData := map[string]string{
		"url":   "https://example.com",
		"alias": "example",
	}
	createBody, _ := json.Marshal(createData)
	resp, err := http.Post(server.URL+"/api/links", "application/json", bytes.NewReader(createBody))
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusCreated {
		fmt.Println("✓ Link created successfully")
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("✗ Failed to create link: %s\n", body)
	}

	// Example 2: Redirect to the link
	fmt.Println("\nRedirecting to 'example'...")
	resp, err = http.Get(server.URL + "/example")
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusFound {
		fmt.Println("✓ Redirect successful")
	} else {
		fmt.Printf("✗ Redirect failed: %d\n", resp.StatusCode)
	}

	// Example 3: List links (with auth)
	fmt.Println("\nListing all links...")
	req, _ := http.NewRequest("GET", server.URL+"/api/links", nil)
	req.Header.Set("Authorization", "Bearer example-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusOK {
		var links []map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&links)
		fmt.Printf("✓ Found %d links\n", len(links))
	} else {
		fmt.Printf("✗ Failed to list links: %d\n", resp.StatusCode)
	}

	// Output:
	// Creating link with alias 'example'...
	// ✓ Link created successfully
	// 
	// Redirecting to 'example'...
	// ✓ Redirect successful
	// 
	// Listing all links...
	// ✓ Found 1 links
}

func TestExampleUsage(t *testing.T) {
	// This test just ensures the example compiles and runs without panicking
	ExampleUsage()
}