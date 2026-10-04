// score measures and scores the contestants' URL shorteners for "Build It in Go" episode 2.
//
//	score measure --src runs/claude-opus-5-5/run1/workspace --out results/claude-opus-5-5/run1 --acceptance ./acceptance
//	score tally   --results results --out results/scorecard.md
//	score blind   --results results --runs-root runs --out blind-review
//
// measure: everything a machine can check (hidden tests, race detector, linters, coverage).
// It writes measure.json and, the first time, findings.json for human verdicts.
// tally: adds the human parts (confirmed findings, blind review) and writes the scorecard.
// blind: copies each run's code to a folder named by a random letter for the blind review.
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
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	var err error
	switch cmd {
	case "measure":
		src := fs.String("src", "", "the contestant's project folder")
		out := fs.String("out", "", "folder for measure.json and findings.json")
		acc := fs.String("acceptance", "acceptance", "folder with the hidden acceptance tests")
		label := fs.String("label", "", "name for this run (default: the last two parts of --out)")
		_ = fs.Parse(args)
		if *src == "" || *out == "" {
			fail("measure needs --src and --out")
		}
		if *label == "" {
			*label = filepath.Join(filepath.Base(filepath.Dir(*out)), filepath.Base(*out))
		}
		accAbs, _ := filepath.Abs(*acc)
		srcAbs, _ := filepath.Abs(*src)
		if err = measure(srcAbs, accAbs, filepath.Join(*out, "measure.json"), *label); err == nil {
			err = seedFindings(*out)
		}
	case "tally":
		results := fs.String("results", "results", "results folder")
		out := fs.String("out", "", "scorecard markdown (default <results>/scorecard.md)")
		_ = fs.Parse(args)
		if *out == "" {
			*out = filepath.Join(*results, "scorecard.md")
		}
		err = tally(*results, *out)
	case "blind":
		results := fs.String("results", "results", "results folder (writes blind/map.json here)")
		runsRoot := fs.String("runs-root", "runs", "folder with <model>/<run>/workspace")
		out := fs.String("out", "blind-review", "folder for the renamed copies")
		_ = fs.Parse(args)
		err = blind(*results, *runsRoot, *out)
	default:
		usage()
	}
	if err != nil {
		fail(err.Error())
	}
}

// seedFindings writes findings.json from measure.json, unless it already exists:
// verdicts typed into it must never be overwritten by a re-measure.
func seedFindings(out string) error {
	path := filepath.Join(out, "findings.json")
	if _, err := os.Stat(path); err == nil {
		logf("%s exists; kept (re-measured tool findings are in measure.json)", path)
		return nil
	}
	var m Measurement
	if err := readJSON(filepath.Join(out, "measure.json"), &m); err != nil {
		return err
	}
	if m.Findings == nil {
		m.Findings = []Finding{}
	}
	return writeJSON(path, m.Findings)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: score measure|tally|blind [flags]")
	os.Exit(2)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "score:", msg)
	os.Exit(1)
}
