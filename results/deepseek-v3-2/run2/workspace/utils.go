package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	codeChars      = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	codeCharsLen   = len(codeChars)
	generatedCodeLen = 7
	maxAttempts    = 1000
)

func generateUniqueCode(existing map[string]*Link) (string, error) {
	for i := 0; i < maxAttempts; i++ {
		code, err := generateRandomCode()
		if err != nil {
			return "", err
		}

		if _, exists := existing[code]; !exists {
			return code, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique code after %d attempts", maxAttempts)
}

func generateRandomCode() (string, error) {
	code := make([]byte, generatedCodeLen)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(codeCharsLen)))
		if err != nil {
			return "", err
		}
		code[i] = codeChars[n.Int64()]
	}
	return string(code), nil
}