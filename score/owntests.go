package main

import (
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// OwnTests is how the project's own tests did.
type OwnTests struct {
	TestFiles int     `json:"test_files"`
	TestFuncs int     `json:"test_funcs"`
	Passed    bool    `json:"passed"`
	Coverage  float64 `json:"coverage"` // percent of statements
	Points    float64 `json:"points"`   // out of 10
	Output    string  `json:"output,omitempty"`
}

// Coverage at or above fullCoverage earns all 10 points; below it, points scale linearly.
const fullCoverage = 80.0

var totalCoveragePattern = regexp.MustCompile(`total:\s+\(statements\)\s+([\d.]+)%`)

// runOwnTests runs the project's tests with coverage. Failing tests earn 0 points.
func runOwnTests(codeDir, workDir string, size ProjectSize) OwnTests {
	result := OwnTests{TestFiles: size.TestFiles, TestFuncs: size.TestFuncs}
	if size.TestFiles == 0 {
		result.Output = "no _test.go files"
		return result
	}
	profile := filepath.Join(workDir, "cover.out")
	tests := run(codeDir, 5*time.Minute, "go", "test", "-count=1", "-timeout", "4m", "-coverprofile", profile, "./...")
	result.Passed = tests.err == nil
	result.Output = truncate(tests.output, 3000)

	if report := run(codeDir, time.Minute, "go", "tool", "cover", "-func", profile); report.err == nil {
		if match := totalCoveragePattern.FindStringSubmatch(report.output); match != nil {
			result.Coverage, _ = strconv.ParseFloat(match[1], 64)
		}
	}
	if result.Passed {
		result.Points = round1(min(10, result.Coverage/fullCoverage*10))
	}
	return result
}
