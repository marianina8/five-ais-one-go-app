package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

func main() {
	// Test what error we get from MaxBytesReader
	largeBody := strings.Repeat("x", 1024*1024+1)
	req := http.Request{
		Body: http.NoBody,
	}
	
	r := http.MaxBytesReader(nil, struct {
		*bytes.Reader
		http.Server
	}{bytes.NewBufferString(largeBody)}, 1024*1024)
	
	buf := make([]byte, 10)
	_, err := r.Read(buf)
	fmt.Printf("Error: %v\n", err)
	fmt.Printf("Error string: %s\n", err.Error())
	if strings.Contains(err.Error(), "request body too large") {
		fmt.Println("Contains 'request body too large'")
	}
}