package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Measurement is everything a machine can say about one run. It holds no score that
// depends on a person; tally adds those from findings.json and the blind review.
type Measurement struct {
	Label       string        `json:"label"` // e.g. claude-sonnet-5-5/run1
	MeasuredAt  time.Time     `json:"measured_at"`
	Size        ProjectSize   `json:"size"`
	Build       Build         `json:"build"`
	Hidden      HiddenResult  `json:"hidden"`
	Style       Style         `json:"style"`
	OwnTests    OwnTests      `json:"own_tests"`
	Findings    []Finding     `json:"findings"` // tool findings; a person gives each a verdict
	Elapsed     time.Duration `json:"elapsed_ns"`
	ScorerNotes []string      `json:"scorer_notes,omitempty"`
}

// Build is the result of go build.
type Build struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
}

// Finding is one possible bug. Source is the tool that reported it, or "agent" for the
// review agent. Verdict starts as "pending"; only "confirmed" findings cost points.
type Finding struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Rule     string `json:"rule,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity,omitempty"` // high|medium|low; agent findings only
	Verdict  string `json:"verdict"`            // pending|confirmed|rejected|duplicate
	Note     string `json:"note,omitempty"`
}

// measure checks one contestant's project and writes the result to outFile.
// It works on a copy, so the project stays exactly as the model left it.
func measure(projectDir, acceptanceDir, outFile, label string) error {
	start := time.Now()
	m := &Measurement{Label: label, MeasuredAt: start.UTC(), Findings: []Finding{}}

	workDir, err := os.MkdirTemp("", "score-*")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			logf("clean up %s: %v", workDir, err)
		}
	}()
	codeDir := filepath.Join(workDir, "code")
	if err := copyProject(projectDir, codeDir); err != nil {
		return fmt.Errorf("copy %s: %w", projectDir, err)
	}
	m.Size = measureSize(codeDir)

	binary := filepath.Join(workDir, "shortener")
	build := run(codeDir, 5*time.Minute, "go", "build", "-o", binary, ".")
	m.Build = Build{OK: build.err == nil, Output: truncate(build.output, 4000)}
	if !m.Build.OK {
		m.ScorerNotes = append(m.ScorerNotes, "does not build: every measured category scores 0")
		m.Hidden = allFailed()
		m.Elapsed = time.Since(start)
		return writeJSON(outFile, m)
	}

	logf("%s: hidden acceptance tests", label)
	if m.Hidden, err = runHiddenTests(acceptanceDir, binary); err != nil {
		return err
	}

	logf("%s: race detector", label)
	m.Findings = append(m.Findings, raceFindings(codeDir, acceptanceDir, workDir)...)

	logf("%s: go vet, staticcheck, gosec, errcheck", label)
	m.Findings = append(m.Findings, bugFindings(codeDir, workDir)...)

	logf("%s: style and complexity", label)
	m.Style = measureStyle(codeDir, workDir)

	logf("%s: its own tests", label)
	m.OwnTests = runOwnTests(codeDir, workDir, m.Size)

	for i := range m.Findings {
		m.Findings[i].ID = fmt.Sprintf("T%02d", i+1)
		m.Findings[i].Verdict = "pending"
	}
	m.Elapsed = time.Since(start)
	return writeJSON(outFile, m)
}
