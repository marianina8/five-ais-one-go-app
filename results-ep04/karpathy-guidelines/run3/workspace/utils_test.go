package main

import (
	"testing"
)

func TestRandomCodeUniqueness(t *testing.T) {
	// Test that generateRandomCode produces valid codes
	code := generateRandomCode(7)
	if len(code) != 7 {
		t.Errorf("expected code length 7, got %d", len(code))
	}

	// Test multiple generations are different (probabilistic)
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := generateRandomCode(7)
		if codes[code] {
			t.Errorf("duplicate code generated: %s", code)
		}
		codes[code] = true
	}
}