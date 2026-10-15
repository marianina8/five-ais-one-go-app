// ep04: prints the Ep04 "5 Claude skills, tested" results for the screen recordings.
//
//	go -C ep04 run . table            the ranking: score /80, change vs baseline, hidden tests, 500-visits test, skill read, cost
//	go -C ep04 run . reads            per run: did the model open its skill?
//	go -C ep04 run . gofunc           fire-and-forget goroutines left in each project's handlers
//	go -C ep04 run . final A B        the model's closing message, arm A next to arm B (run1)
//
// It reads what `score measure` and `score tally` wrote under -results (default ../results-ep04).
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	baselineArm = "baseline"
	visitsTest  = "TestConcurrency_ParallelVisitsExact"
)

type modelScore struct {
	Model                           string
	Works, Bugs, Readable, OwnTests float64
	CostUSD                         float64
}

type stats struct {
	ToolCallsByName map[string]int `json:"tool_calls_by_name"`
	CostUSD         float64        `json:"cost_usd"`
	StoppedBy       string         `json:"stopped_by"`
	FinalText       string         `json:"final_text"`
}

type measure struct {
	Hidden struct {
		Tests map[string]string `json:"tests"`
	} `json:"hidden"`
}

type run struct {
	arm, name string
	dir       string
	stats     stats
	measure   measure
}

func main() {
	results := flag.String("results", "../results-ep04", "folder with <arm>/<run>/ and scorecard.json")
	flag.Parse()
	runs, err := loadRuns(*results)
	if err != nil {
		fail(err)
	}
	switch cmd := flag.Arg(0); cmd {
	case "", "table":
		err = table(*results, runs)
	case "reads":
		reads(runs)
	case "gofunc":
		goFuncs(runs)
	case "final":
		if flag.NArg() < 3 {
			fail(fmt.Errorf("usage: final <armA> <armB>"))
		}
		final(runs, flag.Arg(1), flag.Arg(2))
	default:
		fail(fmt.Errorf("unknown command %q (table, reads, gofunc, final)", cmd))
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ep04:", err)
	os.Exit(1)
}

func loadRuns(dir string) ([]run, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*", "*", "stats.json"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no runs under %s", dir)
	}
	var runs []run
	for _, f := range files {
		runDir := filepath.Dir(f)
		r := run{arm: filepath.Base(filepath.Dir(runDir)), name: filepath.Base(runDir), dir: runDir}
		if err := readJSON(f, &r.stats); err != nil {
			return nil, err
		}
		_ = readJSON(filepath.Join(runDir, "measure.json"), &r.measure) // missing until `score measure` ran
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].arm != runs[j].arm {
			return runs[i].arm < runs[j].arm
		}
		return runs[i].name < runs[j].name
	})
	return runs, nil
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func byArm(runs []run) map[string][]run {
	m := map[string][]run{}
	for _, r := range runs {
		m[r.arm] = append(m[r.arm], r)
	}
	return m
}

func table(dir string, runs []run) error {
	var scores []modelScore
	if err := readJSON(filepath.Join(dir, "scorecard.json"), &scores); err != nil {
		return fmt.Errorf("run `score tally --results %s` first: %w", dir, err)
	}
	arms := byArm(runs)
	total := func(s modelScore) float64 { return s.Works + s.Bugs + s.Readable + s.OwnTests }
	base, haveBase := 0.0, false
	for _, s := range scores {
		if s.Model == baselineArm {
			base, haveBase = total(s), true
		}
	}
	sort.Slice(scores, func(i, j int) bool { return total(scores[i]) > total(scores[j]) })

	fmt.Printf("%-13s %8s %8s %10s %11s %11s %8s\n", "SKILL", "SCORE/80", "VS BASE", "HIDDEN", "500 VISITS", "READ SKILL", "$/RUN")
	fmt.Println(strings.Repeat("─", 76))
	for _, s := range scores {
		rs := arms[s.Model]
		delta := "—"
		if haveBase && s.Model != baselineArm {
			delta = fmt.Sprintf("%+.1f", total(s)-base)
		}
		read := "—"
		if s.Model != baselineArm {
			read = fmt.Sprintf("%d/%d", count(rs, func(r run) bool { return r.stats.ToolCallsByName["Skill"] > 0 }), len(rs))
		}
		fmt.Printf("%-13s %8.1f %8s %10s %11s %11s %8.2f\n", s.Model, total(s), delta, hidden(rs),
			fmt.Sprintf("%d/%d pass", count(rs, func(r run) bool { return r.measure.Hidden.Tests[visitsTest] == "pass" }), len(rs)),
			read, s.CostUSD)
	}
	fmt.Println("\nScore = works 40 + bugs 20 + readable 10 + own tests 10, averaged over runs. HIDDEN = average tests passed.")
	return nil
}

func hidden(rs []run) string {
	pass, tests, n := 0, 0, 0
	for _, r := range rs {
		if len(r.measure.Hidden.Tests) == 0 {
			continue
		}
		n++
		for _, result := range r.measure.Hidden.Tests {
			tests++
			if result == "pass" {
				pass++
			}
		}
	}
	if n == 0 {
		return "no build"
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(pass)/float64(n)), ".0") + fmt.Sprintf("/%.0f", float64(tests)/float64(n))
}

func count(rs []run, ok func(run) bool) int {
	n := 0
	for _, r := range rs {
		if ok(r) {
			n++
		}
	}
	return n
}

func reads(runs []run) {
	fmt.Printf("%-13s %-6s %s\n", "SKILL", "RUN", "OPENED ITS SKILL?")
	for _, r := range runs {
		if r.arm == baselineArm {
			continue
		}
		n := r.stats.ToolCallsByName["Skill"]
		answer := fmt.Sprintf("yes, %d time(s)", n)
		if n == 0 {
			answer = "NO: installed, never read"
		}
		fmt.Printf("%-13s %-6s %s\n", r.arm, r.name, answer)
	}
}

func goFuncs(runs []run) {
	for _, r := range runs {
		files, _ := filepath.Glob(filepath.Join(r.dir, "workspace", "*.go"))
		var hits []string
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			hits = append(hits, scanGoFunc(f)...)
		}
		status := "clean"
		if len(hits) > 0 {
			status = strings.Join(hits, ", ")
		}
		fmt.Printf("%-13s %-6s %s\n", r.arm, r.name, status)
	}
}

// scanGoFunc returns file:line for each `go func` inside an HTTP handler (a func taking http.ResponseWriter).
func scanGoFunc(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var hits []string
	inHandler := false
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(text, "func ") {
			inHandler = strings.Contains(text, "http.ResponseWriter")
		}
		if inHandler && strings.HasPrefix(text, "go func") {
			hits = append(hits, fmt.Sprintf("go func at %s:%d", filepath.Base(path), line))
		}
	}
	return hits
}

func final(runs []run, a, b string) {
	for _, arm := range []string{a, b} {
		for _, r := range runs {
			if r.arm == arm && r.name == "run1" {
				fmt.Printf("── %s (%d words) ──\n%s\n\n", arm, len(strings.Fields(r.stats.FinalText)), strings.TrimSpace(r.stats.FinalText))
			}
		}
	}
}
