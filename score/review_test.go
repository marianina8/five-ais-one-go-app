package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestImportReview(t *testing.T) {
	dir := t.TempDir()
	review := `Some preamble the agent wrote.

### Error handling
- **medium** — store.go:42 — save errors are logged but the 201 is still sent — return 500 instead

### Secrets and sensitive data
- none found

### Resources and concurrency
- **high** — server.go:117 — List returns pointers that are encoded after the lock is released — copy the links [verified]
- **low** — the README is out of date — update it
`
	writeRunFiles(t, dir, review)

	count, err := importReview(dir)
	if err != nil || count != 3 {
		t.Fatalf("count %d, err %v", count, err)
	}
	var findings []Finding
	if err := readJSON(filepath.Join(dir, "findings.json"), &findings); err != nil {
		t.Fatal(err)
	}
	if len(findings) != 4 || findings[0].Verdict != "rejected" {
		t.Fatalf("existing verdicts must be kept: %+v", findings)
	}
	want := Finding{ID: "A02", Source: "agent", Rule: "Resources and concurrency", File: "server.go", Line: 117, Severity: "high", Verdict: "pending",
		Message: "server.go:117 — List returns pointers that are encoded after the lock is released — copy the links [verified]"}
	if !reflect.DeepEqual(findings[2], want) || findings[3].File != "" {
		t.Errorf("parsed %+v and %+v", findings[2], findings[3])
	}
	if _, err := importReview(dir); err == nil {
		t.Error("a second import must be refused, so verdicts are never overwritten")
	}
}

func writeRunFiles(t *testing.T, dir, review string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte(review), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "findings.json"), []Finding{{ID: "T01", Source: "errcheck", Verdict: "rejected"}}); err != nil {
		t.Fatal(err)
	}
}
