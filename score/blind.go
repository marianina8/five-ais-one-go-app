package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Words that would give away which model wrote a project during the blind review.
var giveaways = regexp.MustCompile(`(?i)\b(claude|anthropic|sonnet|opus|gpt|openai|chatgpt|gemini|google|deepseek|qwen|llm|ai[- ]generated)\b`)

// blind copies every run's project to out/<letter>/ in random order and records the mapping
// in results/blind/map.json. Only the code goes over: no logs, stats or model names.
func blind(results, runsRoot, out string) error {
	mapPath := filepath.Join(results, "blind", "map.json")
	if _, err := os.Stat(mapPath); err == nil {
		return fmt.Errorf("%s already exists; delete it to make a new blind set", mapPath)
	}
	workspaces, _ := filepath.Glob(filepath.Join(runsRoot, "*", "*", "workspace"))
	if len(workspaces) == 0 {
		return fmt.Errorf("no %s/<model>/<run>/workspace folders", runsRoot)
	}
	if len(workspaces) > 26 {
		return fmt.Errorf("%d runs; at most 26 letters", len(workspaces))
	}
	// Fisher–Yates shuffle with crypto/rand.
	for i := len(workspaces) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		workspaces[i], workspaces[j.Int64()] = workspaces[j.Int64()], workspaces[i]
	}
	mapping := map[string]string{}
	var warnings []string
	for i, ws := range workspaces {
		letter := string(rune('A' + i))
		run := filepath.Base(filepath.Dir(ws))
		model := filepath.Base(filepath.Dir(filepath.Dir(ws)))
		mapping[letter] = model + "/" + run
		dst := filepath.Join(out, letter)
		if err := copyGoProject(ws, dst); err != nil {
			return err
		}
		_ = filepath.Walk(dst, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if ext := filepath.Ext(p); ext != ".go" && ext != ".md" && info.Name() != "go.mod" {
				return os.Remove(p) // data files, binaries: not part of the review
			}
			b, _ := os.ReadFile(p)
			for n, line := range strings.Split(string(b), "\n") {
				if giveaways.MatchString(line) {
					rel, _ := filepath.Rel(out, p)
					warnings = append(warnings, fmt.Sprintf("%s:%d mentions %q", rel, n+1, giveaways.FindString(line)))
				}
			}
			return nil
		})
	}
	if err := writeJSON(mapPath, mapping); err != nil {
		return err
	}
	letters := make([]string, 0, len(mapping))
	for l := range mapping {
		letters = append(letters, l)
	}
	sort.Strings(letters)
	reviews := map[string]Review{}
	for _, l := range letters {
		reviews[l] = Review{}
	}
	reviewPath := filepath.Join(results, "blind", "reviews.json")
	if _, err := os.Stat(reviewPath); os.IsNotExist(err) {
		if err := writeJSON(reviewPath, reviews); err != nil {
			return err
		}
	}
	fmt.Printf("Wrote %d projects to %s/ (%s). Mapping sealed in %s: don't open it until the reveal.\n",
		len(letters), out, strings.Join(letters, ", "), mapPath)
	if len(warnings) > 0 {
		fmt.Println("\nThese lines could give a model away. Check them before you start the review:")
		for _, w := range warnings {
			fmt.Println("  " + w)
		}
	}
	return nil
}
