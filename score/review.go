package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The review agent writes one bullet per problem under seven headings:
//
//	### Resources and concurrency
//	- **high** — store.go:116 — what is wrong and why — how to fix it [verified]
var (
	reviewHeading  = regexp.MustCompile(`^###\s+(.+?)\s*$`)
	reviewBullet   = regexp.MustCompile(`^\s*-\s+\*\*(high|medium|low)\*\*\s+[—–-]+\s+(.+)$`)
	reviewLocation = regexp.MustCompile(`^([\w./-]+\.go)(?::(\d+))?`)
)

// importReview adds the review agent's findings from <runDir>/review.md to
// <runDir>/findings.json as pending agent findings (A01, A02, ...). It refuses to run twice,
// so verdicts already typed in are never overwritten.
func importReview(runDir string) (int, error) {
	findingsFile := filepath.Join(runDir, "findings.json")
	var findings []Finding
	if err := readJSON(findingsFile, &findings); err != nil {
		return 0, fmt.Errorf("%w (run score measure first)", err)
	}
	for _, f := range findings {
		if f.Source == "agent" {
			return 0, fmt.Errorf("%s already has the agent's findings", findingsFile)
		}
	}
	review, err := os.Open(filepath.Join(runDir, "review.md")) // #nosec G304 -- a file in the folder the operator passed in
	if err != nil {
		return 0, err
	}
	defer func() { _ = review.Close() }()

	agentFindings, err := parseReview(review)
	if err != nil {
		return 0, err
	}
	return len(agentFindings), writeJSON(findingsFile, append(findings, agentFindings...))
}

// parseReview reads the review's bullets. Text outside the format (a preamble, "none found")
// is skipped.
func parseReview(review *os.File) ([]Finding, error) {
	var findings []Finding
	heading := ""
	scanner := bufio.NewScanner(review)
	for scanner.Scan() {
		line := scanner.Text()
		if match := reviewHeading.FindStringSubmatch(line); match != nil {
			heading = match[1]
			continue
		}
		match := reviewBullet.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		finding := Finding{
			ID:       fmt.Sprintf("A%02d", len(findings)+1),
			Source:   "agent",
			Rule:     heading,
			Severity: match[1],
			Message:  strings.TrimSpace(match[2]),
			Verdict:  "pending",
		}
		if location := reviewLocation.FindStringSubmatch(finding.Message); location != nil {
			finding.File = location[1]
			finding.Line, _ = strconv.Atoi(location[2])
		}
		findings = append(findings, finding)
	}
	return findings, scanner.Err()
}
