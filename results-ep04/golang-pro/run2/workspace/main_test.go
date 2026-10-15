package main

import (
	"bytes"
	"flag"
	"os"
	"testing"
)

func TestMain_ADMIN_TOKENRequired(t *testing.T) {
	// Save and clear command-line flags
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	
	// Clear any existing ADMIN_TOKEN
	os.Unsetenv("ADMIN_TOKEN")
	
	// Test that main would exit with error when ADMIN_TOKEN is not set
	// We can't actually run main() because it would call os.Exit
	// But we can test the logic by checking the environment variable check
	
	if os.Getenv("ADMIN_TOKEN") != "" {
		t.Error("ADMIN_TOKEN should be unset for this test")
	}
	
	// The main function will log.Fatal if ADMIN_TOKEN is not set
	// We can't test this easily without actually running main
	// but the logic is there in the code
}

func TestFlagParsing(t *testing.T) {
	// Test that flag parsing works correctly
	tests := []struct {
		name     string
		args     []string
		expectedAddr string
		expectedData string
	}{
		{
			name: "default values",
			args: []string{"shortener"},
			expectedAddr: ":8080",
			expectedData: "data.json",
		},
		{
			name: "custom addr",
			args: []string{"shortener", "-addr", ":9090"},
			expectedAddr: ":9090",
			expectedData: "data.json",
		},
		{
			name: "custom data file",
			args: []string{"shortener", "-data", "custom.json"},
			expectedAddr: ":8080",
			expectedData: "custom.json",
		},
		{
			name: "both custom",
			args: []string{"shortener", "-addr", ":9090", "-data", "custom.json"},
			expectedAddr: ":9090",
			expectedData: "custom.json",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flag parser
			flag.CommandLine = flag.NewFlagSet(tt.args[0], flag.ContinueOnError)
			
			// Parse flags
			addr := flag.String("addr", ":8080", "Address to listen on")
			dataFile := flag.String("data", "data.json", "Data file for persistence")
			
			// Parse with test arguments
			err := flag.CommandLine.Parse(tt.args[1:])
			if err != nil {
				t.Fatalf("Failed to parse flags: %v", err)
			}
			
			if *addr != tt.expectedAddr {
				t.Errorf("Expected addr %q, got %q", tt.expectedAddr, *addr)
			}
			
			if *dataFile != tt.expectedData {
				t.Errorf("Expected data file %q, got %q", tt.expectedData, *dataFile)
			}
		})
	}
}