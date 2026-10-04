package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed lint-bugs.yml
var bugsLintConfig []byte

//go:embed lint-style.yml
var styleLintConfig []byte

//go:embed lint-cyclo.yml
var complexityLintConfig []byte

// Style is the input to the "Readable Go" ranking.
type Style struct {
	Measured        bool           `json:"measured"` // false if golangci-lint failed; then Score means nothing
	Issues          int            `json:"issues"`
	IssuesByLinter  map[string]int `json:"issues_by_linter"`
	MaxComplexity   int            `json:"max_complexity"`
	MostComplexFunc string         `json:"most_complex_func"`
	Score           int            `json:"score"` // issues + max(0, max_complexity-10); lower is better
	Details         []string       `json:"details"`
}

type lintIssue struct {
	FromLinter string
	Text       string
	Pos        struct {
		Filename string
		Line     int
	}
}

// golangciLint runs golangci-lint with one of the embedded configs and returns its issues.
func golangciLint(codeDir, workDir, name string, config []byte) ([]lintIssue, error) {
	configFile := filepath.Join(workDir, "lint-"+name+".yml")
	if err := os.WriteFile(configFile, config, 0o600); err != nil {
		return nil, err
	}
	result := run(codeDir, 5*time.Minute, "golangci-lint", "run", "--config", configFile,
		"--output.json.path=stdout", "--output.text.path=stderr", "--show-stats=false", "./...")
	// The JSON report is the first line of stdout.
	report, _, _ := strings.Cut(result.stdout, "\n")
	var parsed struct{ Issues []lintIssue }
	if err := json.Unmarshal([]byte(report), &parsed); err != nil {
		return nil, fmt.Errorf("golangci-lint (%s): %w\n%s", name, err, truncate(result.output, 1000))
	}
	return parsed.Issues, nil
}

var ruleIDPattern = regexp.MustCompile(`^(G\d{3}|SA\d{4}):?`)

// bugFindings runs go vet, staticcheck's bug checks, gosec and errcheck.
func bugFindings(codeDir, workDir string) []Finding {
	issues, err := golangciLint(codeDir, workDir, "bugs", bugsLintConfig)
	if err != nil {
		return []Finding{{Source: "scorer", Message: err.Error()}}
	}
	var findings []Finding
	for _, issue := range issues {
		source := issue.FromLinter
		if source == "govet" {
			source = "go vet"
		}
		var rule string
		if match := ruleIDPattern.FindStringSubmatch(issue.Text); match != nil {
			rule = match[1]
		}
		findings = append(findings, Finding{Source: source, Rule: rule, File: issue.Pos.Filename, Line: issue.Pos.Line, Message: issue.Text})
	}
	return findings
}

var complexityPattern = regexp.MustCompile(`cyclomatic complexity (\d+) of func (\S+)`)

// measureStyle counts style issues and finds the most complex function.
func measureStyle(codeDir, workDir string) Style {
	style := Style{IssuesByLinter: map[string]int{}}
	issues, err := golangciLint(codeDir, workDir, "style", styleLintConfig)
	if err != nil {
		style.Details = append(style.Details, err.Error())
		return style
	}
	for _, issue := range issues {
		style.Issues++
		style.IssuesByLinter[issue.FromLinter]++
		style.Details = append(style.Details, fmt.Sprintf("%s:%d %s (%s)", issue.Pos.Filename, issue.Pos.Line, issue.Text, issue.FromLinter))
	}

	functions, err := golangciLint(codeDir, workDir, "complexity", complexityLintConfig)
	if err != nil {
		style.Details = append(style.Details, err.Error())
		return style
	}
	for _, function := range functions {
		match := complexityPattern.FindStringSubmatch(function.Text)
		if match == nil {
			continue
		}
		if complexity, _ := strconv.Atoi(match[1]); complexity > style.MaxComplexity {
			style.MaxComplexity, style.MostComplexFunc = complexity, match[2]
		}
	}
	style.Score = style.Issues + max(0, style.MaxComplexity-10)
	style.Measured = true
	return style
}
