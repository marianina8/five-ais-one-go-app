package main

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// giveawayPattern matches words that would reveal which model wrote a project.
var giveawayPattern = regexp.MustCompile(`(?i)\b(claude|anthropic|sonnet|opus|gpt|openai|chatgpt|gemini|google|deepseek|qwen|llm|ai[- ]generated)\b`)

// blind copies every run's project to reviewDir/<letter>/ in random order and records which
// letter is which run in resultsDir/blind/map.json. Only the code is copied: no logs, stats or
// data files. It also creates an empty reviews.json to fill in, one entry per letter.
func blind(resultsDir, runsDir, reviewDir string) error {
	mapFile := filepath.Join(resultsDir, "blind", "map.json")
	projects, err := blindCandidates(runsDir, reviewDir, mapFile)
	if err != nil {
		return err
	}

	letterToRun := map[string]string{}
	var letters, giveaways []string
	for i, project := range projects {
		letter := string(rune('A' + i))
		runDir := filepath.Dir(project)
		letterToRun[letter] = filepath.Base(filepath.Dir(runDir)) + "/" + filepath.Base(runDir)
		letters = append(letters, letter)

		copyDir := filepath.Join(reviewDir, letter)
		if err := copyProject(project, copyDir); err != nil {
			return err
		}
		found, err := keepCodeOnly(copyDir, reviewDir)
		if err != nil {
			return err
		}
		giveaways = append(giveaways, found...)
	}
	if err := writeBlindFiles(resultsDir, letterToRun); err != nil {
		return err
	}

	fmt.Printf("Wrote %d projects to %s/ (%s).\nThe mapping is in %s: don't open it until the reveal.\n",
		len(letters), reviewDir, strings.Join(letters, ", "), mapFile)
	if len(giveaways) > 0 {
		fmt.Println("\nThese lines could give a model away. Check them before you start the review:")
		for _, line := range giveaways {
			fmt.Println("  " + line)
		}
	}
	return nil
}

// blindCandidates finds every run's project, in random order, after checking that no
// earlier blind set would be overwritten or mixed in.
func blindCandidates(runsDir, reviewDir, mapFile string) ([]string, error) {
	reviewsFile := filepath.Join(filepath.Dir(mapFile), "reviews.json")
	for _, earlier := range []string{mapFile, reviewsFile} {
		if _, err := os.Stat(earlier); err == nil {
			return nil, fmt.Errorf("%s already exists; a new blind set reshuffles the letters, so delete it first", earlier)
		}
	}
	if _, err := os.Stat(reviewDir); err == nil {
		return nil, fmt.Errorf("%s already exists; delete it so no old projects mix in", reviewDir)
	}
	projects, err := filepath.Glob(filepath.Join(runsDir, "*", "*", "workspace"))
	if err != nil {
		return nil, err
	}
	switch {
	case len(projects) == 0:
		return nil, fmt.Errorf("no %s/<model>/<run>/workspace folders", runsDir)
	case len(projects) > 26:
		return nil, fmt.Errorf("%d runs, but there are only 26 letters", len(projects))
	}
	return projects, shuffle(projects)
}

// writeBlindFiles saves the letter mapping and an empty review sheet.
func writeBlindFiles(resultsDir string, letterToRun map[string]string) error {
	if err := writeJSON(filepath.Join(resultsDir, "blind", "map.json"), letterToRun); err != nil {
		return err
	}
	reviewsFile := filepath.Join(resultsDir, "blind", "reviews.json")
	emptyReviews := map[string]Review{}
	for letter := range letterToRun {
		emptyReviews[letter] = Review{}
	}
	return writeJSON(reviewsFile, emptyReviews)
}

// shuffle puts paths in random order (Fisher–Yates with crypto/rand).
func shuffle(paths []string) error {
	for i := len(paths) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		paths[i], paths[j.Int64()] = paths[j.Int64()], paths[i]
	}
	return nil
}

// keepCodeOnly deletes everything in dir except .go, .md and go.mod files, and returns the
// lines that mention a model or vendor, as "path:line mentions ...", relative to baseDir.
func keepCodeOnly(dir, baseDir string) ([]string, error) {
	var giveaways []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext != ".go" && ext != ".md" && entry.Name() != "go.mod" {
			return os.Remove(path) // data files and binaries aren't part of the review
		}
		source, err := os.ReadFile(path) // #nosec G304 -- reads a file under a folder the operator passed in
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(baseDir, path)
		for number, line := range strings.Split(string(source), "\n") {
			if word := giveawayPattern.FindString(line); word != "" {
				giveaways = append(giveaways, fmt.Sprintf("%s:%d mentions %q", rel, number+1, word))
			}
		}
		return nil
	})
	return giveaways, err
}
