package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRFC3339(t *testing.T) {
	l := Link{CreatedAt: time.Now().UTC()}
	b, _ := json.Marshal(l)
	if !strings.Contains(string(b), "Z") && !strings.Contains(string(b), "+") {
		t.Errorf("Does not look like RFC3339: %s", string(b))
	}
}
