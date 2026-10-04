package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CategoryPoints splits the 40 "Does it work?" points across the hidden test categories,
// so a category with many small cases (Validation) can't outweigh one with a few hard ones.
var CategoryPoints = map[string]float64{
	"Core":        8,
	"Validation":  8,
	"Admin":       7,
	"Errors":      3,
	"Persistence": 7,
	"Concurrency": 7,
}

// HiddenResult is how a project did on the hidden acceptance tests.
type HiddenResult struct {
	Tests      map[string]string  `json:"tests"`      // leaf test -> pass|fail
	Categories map[string][2]int  `json:"categories"` // category -> [passed, total]
	Points     map[string]float64 `json:"points"`     // category -> points
	Total      float64            `json:"total"`      // out of 40
	Failures   map[string]string  `json:"failures"`   // leaf test -> why it failed
}

// allTests lists every leaf test, taken from a run against the reference. It lets a suite
// that crashed or timed out still count each test that never reported as a failure.
//
//go:embed tests.txt
var allTestsFile string

func allTests() []string {
	var names []string
	for _, line := range strings.Split(allTestsFile, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// runHiddenTests runs the whole acceptance suite against binary and scores it.
func runHiddenTests(acceptanceDir, binary string) (HiddenResult, error) {
	outcomes, outputs, err := goTestJSON(acceptanceDir, binary, "", nil)
	if err != nil {
		return HiddenResult{}, err
	}
	for _, name := range allTests() {
		if _, reported := outcomes[name]; !reported {
			outcomes[name] = "fail"
		}
	}
	return scoreHidden(outcomes, outputs), nil
}

// allFailed is the result for a project that doesn't build.
func allFailed() HiddenResult {
	outcomes := map[string]string{}
	for _, name := range allTests() {
		outcomes[name] = "fail"
	}
	result := scoreHidden(outcomes, nil)
	result.Failures = map[string]string{"*": "does not build"}
	return result
}

// goTestJSON runs `go test -json` on the acceptance suite and returns each leaf test's
// outcome and output. runPattern limits the tests (go test -run); extraEnv is added to
// the minimal environment. If no test ran at all (say the suite didn't compile or the
// folder is wrong), that's a scorer problem, so it returns an error rather than failures.
func goTestJSON(acceptanceDir, binary, runPattern string, extraEnv []string) (outcomes, outputs map[string]string, err error) {
	args := []string{"test", "-count=1", "-json", "-timeout", "15m"}
	if runPattern != "" {
		args = append(args, "-run", runPattern)
	}
	args = append(args, "./...")
	env := append([]string{"SHORTENER_BIN=" + binary}, extraEnv...)
	result := runWithEnv(acceptanceDir, 20*time.Minute, env, "go", args...)

	outcomes, outputs = parseTestEvents(result.output)
	if len(outcomes) == 0 && result.err != nil {
		return nil, nil, fmt.Errorf("hidden tests did not run: %v\n%s", result.err, truncate(result.output, 1000))
	}
	return outcomes, outputs, nil
}

// parseTestEvents reads `go test -json` output. It keeps leaf tests only: a test with
// subtests just sums them up.
func parseTestEvents(jsonLines string) (outcomes, outputs map[string]string) {
	outcomes = map[string]string{}
	lines := map[string][]string{}
	scanner := bufio.NewScanner(strings.NewReader(jsonLines))
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		var event struct{ Action, Test, Output string }
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Test == "" {
			continue
		}
		switch event.Action {
		case "pass", "fail", "skip":
			outcomes[event.Test] = event.Action
		case "output":
			lines[event.Test] = append(lines[event.Test], event.Output)
		}
	}
	for name := range outcomes {
		if i := strings.LastIndex(name, "/"); i > 0 {
			delete(outcomes, name[:i])
		}
	}
	outputs = map[string]string{}
	for name, output := range lines {
		outputs[name] = strings.Join(output, "")
	}
	return outcomes, outputs
}

// scoreHidden turns test outcomes into category points.
func scoreHidden(outcomes, outputs map[string]string) HiddenResult {
	result := HiddenResult{
		Tests:      outcomes,
		Categories: map[string][2]int{},
		Points:     map[string]float64{},
		Failures:   map[string]string{},
	}
	for name, outcome := range outcomes {
		category := testCategory(name)
		counts := result.Categories[category]
		counts[1]++
		if outcome == "pass" {
			counts[0]++
		} else if output, ok := outputs[name]; ok {
			result.Failures[name] = failureReason(output)
		} else {
			result.Failures[name] = "did not finish (the suite timed out or crashed)"
		}
		result.Categories[category] = counts
	}
	for category, points := range CategoryPoints {
		if counts := result.Categories[category]; counts[1] > 0 {
			result.Points[category] = round1(points * float64(counts[0]) / float64(counts[1]))
			result.Total += result.Points[category]
		}
	}
	result.Total = round1(result.Total)
	return result
}

// testCategory is the part of a test name between "Test" and the first "_".
func testCategory(name string) string {
	name = strings.TrimPrefix(name, "Test")
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name[:i]
	}
	return "Other"
}

// failureReason keeps the lines a failing test printed, without go test's own headers.
func failureReason(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "=== ") || strings.HasPrefix(line, "--- ") {
			continue
		}
		lines = append(lines, line)
	}
	return truncate(strings.Join(lines, " | "), 400)
}
