// Command score measures and scores the contestants' URL shorteners.
//
//	score measure --project runs/<model>/<run>/workspace --out results/<model>/<run>
//	score blind   --runs runs --results results --review blind-review
//	score tally   --results results
//
// measure checks everything a machine can (hidden tests, race detector, linters, coverage)
// and writes measure.json, plus findings.json for a person's verdicts the first time.
// blind copies each project to a random letter for the blind review.
// tally adds the human parts (confirmed findings, blind review) and writes the scorecard.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	command, args := os.Args[1], os.Args[2:]
	var err error
	switch command {
	case "measure":
		err = measureCommand(args)
	case "blind":
		err = blindCommand(args)
	case "tally":
		err = tallyCommand(args)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "score:", err)
		os.Exit(1)
	}
}

func measureCommand(args []string) error {
	flags := flag.NewFlagSet("measure", flag.ExitOnError)
	projectDir := flags.String("project", "", "the contestant's project folder")
	outDir := flags.String("out", "", "folder for measure.json and findings.json, e.g. results/<model>/<run>")
	acceptanceDir := flags.String("acceptance", "acceptance", "folder with the hidden acceptance tests")
	label := flags.String("label", "", "name for this run (default: the last two parts of --out)")
	_ = flags.Parse(args) // ExitOnError: Parse exits on a bad flag

	if *projectDir == "" || *outDir == "" {
		return fmt.Errorf("measure needs --project and --out")
	}
	if *label == "" {
		*label = filepath.Base(filepath.Dir(*outDir)) + "/" + filepath.Base(*outDir)
	}
	project, err := filepath.Abs(*projectDir)
	if err != nil {
		return err
	}
	acceptance, err := filepath.Abs(*acceptanceDir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(acceptance, "acceptance_test.go")); err != nil {
		return fmt.Errorf("no hidden tests in %s (set --acceptance to the repo's acceptance folder)", acceptance)
	}
	if err := measure(project, acceptance, filepath.Join(*outDir, "measure.json"), *label); err != nil {
		return err
	}
	return seedFindings(*outDir)
}

func blindCommand(args []string) error {
	flags := flag.NewFlagSet("blind", flag.ExitOnError)
	runsDir := flags.String("runs", "runs", "folder with <model>/<run>/workspace")
	resultsDir := flags.String("results", "results", "results folder; the mapping goes in <results>/blind/")
	reviewDir := flags.String("review", "blind-review", "folder for the projects renamed by letter")
	_ = flags.Parse(args)
	return blind(*resultsDir, *runsDir, *reviewDir)
}

func tallyCommand(args []string) error {
	flags := flag.NewFlagSet("tally", flag.ExitOnError)
	resultsDir := flags.String("results", "results", "results folder")
	scorecard := flags.String("out", "", "scorecard Markdown file (default <results>/scorecard.md)")
	_ = flags.Parse(args)
	if *scorecard == "" {
		*scorecard = filepath.Join(*resultsDir, "scorecard.md")
	}
	return tally(*resultsDir, *scorecard)
}

// seedFindings copies the tool findings from measure.json to findings.json, unless
// findings.json already exists: verdicts typed into it must survive a re-measure.
func seedFindings(outDir string) error {
	findingsFile := filepath.Join(outDir, "findings.json")
	if _, err := os.Stat(findingsFile); err == nil {
		logf("%s exists; kept it (the re-measured tool findings are in measure.json)", findingsFile)
		return nil
	}
	var m Measurement
	if err := readJSON(filepath.Join(outDir, "measure.json"), &m); err != nil {
		return err
	}
	return writeJSON(findingsFile, m.Findings)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: score measure|blind|tally [flags]   (score <command> -h for flags)")
	os.Exit(2)
}
