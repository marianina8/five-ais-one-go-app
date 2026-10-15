package main

import (
	"strings"
	"testing"
)

func TestValidateAliasDebug(t *testing.T) {
	tests := []struct {
		alias string
		valid bool
	}{
		{"ab", false},          // too short
		{"abc", true},          // exactly 3 chars
		{"valid-alias_123", true}, // valid with - and _
		{"my page", false},     // space not allowed
		{"api", false},         // reserved
		{strings.Repeat("a", 33), false}, // too long
	}
	
	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			err := ValidateAlias(tt.alias)
			if tt.valid && err != nil {
				t.Errorf("expected alias %q to be valid, got error: %v", tt.alias, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected alias %q to be invalid, got no error", tt.alias)
			}
		})
	}
}