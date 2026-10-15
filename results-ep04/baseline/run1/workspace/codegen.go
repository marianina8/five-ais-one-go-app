package main

import (
	"math/rand"
	"time"
)

const (
	codeLength      = 7
	charset         = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	charCount       = len(charset)
	maxAttempts     = 1000 // Safety limit for generation attempts
)

type codeGenerator struct {
	rng *rand.Rand
	used map[string]bool
}

func newCodeGenerator() *codeGenerator {
	return &codeGenerator{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		used: make(map[string]bool),
	}
}

func (cg *codeGenerator) generate() string {
	for attempts := 0; attempts < maxAttempts; attempts++ {
		code := make([]byte, codeLength)
		for i := range code {
			code[i] = charset[cg.rng.Intn(charCount)]
		}
		strCode := string(code)
		if !cg.used[strCode] {
			return strCode
		}
	}
	// Fallback: add timestamp to ensure uniqueness
	return cg.generateWithTimestamp()
}

func (cg *codeGenerator) generateWithTimestamp() string {
	// This should almost never happen, but just in case
	base := make([]byte, codeLength)
	for i := range base {
		base[i] = charset[cg.rng.Intn(charCount)]
	}
	
	timestamp := time.Now().UnixNano()
	// Mix timestamp with random base
	result := make([]byte, codeLength)
	for i := 0; i < codeLength; i++ {
		// Use bits from timestamp to modify the character
		charIdx := int(base[i]) + int((timestamp>>(i*8))&0xFF)
		result[i] = charset[charIdx%charCount]
	}
	return string(result)
}

func (cg *codeGenerator) markUsed(code string) {
	cg.used[code] = true
}

func (cg *codeGenerator) unmarkUsed(code string) {
	delete(cg.used, code)
}