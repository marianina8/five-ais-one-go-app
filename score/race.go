package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// raceTests are the hidden tests that drive a -race build: everything concurrent,
// plus the basic paths those requests go through.
const raceTests = "TestConcurrency|TestCore|TestAdmin_Delete"

// raceFindings builds the project with -race, drives it with the hidden tests in raceTests,
// and runs the project's own tests with -race. Each distinct race becomes one finding.
func raceFindings(codeDir, acceptanceDir, workDir string) []Finding {
	cgo := []string{"CGO_ENABLED=1"} // the race detector needs cgo
	raceBinary := filepath.Join(workDir, "shortener-race")
	if build := runWithEnv(codeDir, 5*time.Minute, cgo, "go", "build", "-race", "-o", raceBinary, "."); build.err != nil {
		return []Finding{{Source: "race", Message: "could not build with -race: " + truncate(build.output, 300)}}
	}

	// GORACE log_path makes the program write each report to a file instead of stderr.
	logDir := filepath.Join(workDir, "race-logs")
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return []Finding{{Source: "scorer", Message: "race log folder: " + err.Error()}}
	}
	// Only the race reports matter here, not which tests pass.
	if _, _, err := goTestJSON(acceptanceDir, raceBinary, raceTests, []string{"GORACE=log_path=" + filepath.Join(logDir, "report") + " halt_on_error=0"}); err != nil {
		return []Finding{{Source: "scorer", Message: "race run: " + err.Error()}}
	}

	var reports []string
	logFiles, _ := filepath.Glob(filepath.Join(logDir, "report*"))
	for _, file := range logFiles {
		data, err := os.ReadFile(file) // #nosec G304 -- a log file this function created
		if err != nil {
			continue
		}
		reports = append(reports, splitRaceReports(string(data))...)
	}
	ownTests := runWithEnv(codeDir, 5*time.Minute, cgo, "go", "test", "-race", "-count=1", "./...")
	reports = append(reports, splitRaceReports(ownTests.output)...)

	var findings []Finding
	seen := map[string]bool{}
	for _, report := range reports {
		key, file, line := raceLocation(report)
		if seen[key] {
			continue
		}
		seen[key] = true
		findings = append(findings, Finding{Source: "race", Rule: "DATA RACE", File: file, Line: line, Message: truncate(report, 1500)})
	}
	return findings
}

func splitRaceReports(output string) []string {
	const header = "WARNING: DATA RACE"
	parts := strings.Split(output, header)
	var reports []string
	for _, part := range parts[1:] {
		if end := strings.Index(part, "=================="); end >= 0 {
			part = part[:end]
		}
		reports = append(reports, header+part)
	}
	return reports
}

var stackFramePattern = regexp.MustCompile(`(?m)^\s+(/\S+\.go):(\d+)`)

// raceLocation names a race by the first two places in the project's own code that it
// touches, so the same race reported twice counts once. file and line are the first place.
func raceLocation(report string) (key, file string, line int) {
	goroot := goEnv("GOROOT")
	var places []string
	for _, frame := range stackFramePattern.FindAllStringSubmatch(report, -1) {
		path, lineNumber := frame[1], frame[2]
		if strings.HasPrefix(path, goroot) || strings.Contains(path, "/pkg/mod/") {
			continue // the standard library or a dependency, not the project
		}
		if file == "" {
			file = filepath.Base(path)
			line, _ = strconv.Atoi(lineNumber)
		}
		places = append(places, filepath.Base(path)+":"+lineNumber)
		if len(places) == 2 {
			break
		}
	}
	slices.Sort(places)
	return strings.Join(places, "|"), file, line
}
