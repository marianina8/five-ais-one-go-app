package main

import (
	"math/rand"
	"time"
)

const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
const codeLength = 7

var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// generateRandomCode generates a random code of specified length
func generateRandomCode(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = letters[seededRand.Intn(len(letters))]
	}
	return string(b)
}

// init seeds the random number generator
func init() {
	rand.Seed(time.Now().UnixNano())
}