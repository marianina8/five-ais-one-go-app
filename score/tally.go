package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Results layout (one folder per run):
//
//	results/<model>/<run>/measure.json   machine measurements (score measure)
//	results/<model>/<run>/findings.json  tool + review-agent findings with human verdicts
//	results/<model>/<run>/stats.json     harness log: model calls, tokens, cost, time
//	results/blind/map.json               {"A": "<model>/<run>", ...}  (sealed until the reveal)
//	results/blind/reviews.json           {"A": {"approve": 0-10, "understand": 0-10, "notes": ""}}

// Deductions for confirmed findings (Bugs and security starts at 20, floor 0).
var agentDeduction = map[string]float64{"high": 4, "medium": 2, "low": 1}

const toolDeduction = 2

type Stats struct {
	ModelCalls   int     `json:"model_calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Seconds      float64 `json:"seconds"`
	Finished     bool    `json:"finished"` // the model said DONE (vs. ran out of steps or time)
}

type Review struct {
	Approve    float64 `json:"approve"`
	Understand float64 `json:"understand"`
	Notes      string  `json:"notes"`
}

type RunScore struct {
	Model, Run     string
	Built          bool
	Works, Bugs    float64
	Tests          float64
	Merge          float64
	Reviewed       bool
	StyleRaw       int
	Confirmed      []Finding
	Pending        int
	Stats          Stats
	HiddenPassed   int
	HiddenTotal    int
	Coverage       float64
	OwnTestsPassed bool
}

type ModelScore struct {
	Model                        string
	Runs                         []*RunScore
	Works, Bugs, Readable, Tests float64
	Merge                        float64
	Total                        float64
	StyleRaw                     float64 // average over runs that built
	Built                        bool    // at least one run built
	ConfirmedHigh                int
	Cost, Seconds                float64
	Calls                        float64
}

func tally(results, outMD string) error {
	runDirs, _ := filepath.Glob(filepath.Join(results, "*", "*", "measure.json"))
	if len(runDirs) == 0 {
		return fmt.Errorf("no %s/<model>/<run>/measure.json files", results)
	}
	blindMap := map[string]string{}
	reviews := map[string]Review{}
	_ = readJSON(filepath.Join(results, "blind", "map.json"), &blindMap)
	_ = readJSON(filepath.Join(results, "blind", "reviews.json"), &reviews)
	reviewOf := map[string]Review{}
	for letter, run := range blindMap {
		if r, ok := reviews[letter]; ok {
			reviewOf[run] = r
		}
	}

	models := map[string]*ModelScore{}
	var warnings []string
	for _, mp := range runDirs {
		dir := filepath.Dir(mp)
		run := filepath.Base(dir)
		model := filepath.Base(filepath.Dir(dir))
		var m Measurement
		if err := readJSON(mp, &m); err != nil {
			return fmt.Errorf("%s: %w", mp, err)
		}
		var findings []Finding
		if err := readJSON(filepath.Join(dir, "findings.json"), &findings); err != nil {
			findings = m.Findings
		}
		var st Stats
		_ = readJSON(filepath.Join(dir, "stats.json"), &st)

		rs := &RunScore{Model: model, Run: run, Built: m.Build.OK, Stats: st, Coverage: m.OwnTests.Coverage, OwnTestsPassed: m.OwnTests.Passed}
		for _, c := range m.Hidden.Categories {
			rs.HiddenPassed += c[0]
			rs.HiddenTotal += c[1]
		}
		if m.Build.OK {
			rs.Works = m.Hidden.Total
			rs.Tests = m.OwnTests.Points
			rs.StyleRaw = m.Style.Raw
			bugs := 20.0
			for _, f := range findings {
				switch f.Verdict {
				case "confirmed":
					rs.Confirmed = append(rs.Confirmed, f)
					if f.Source == "agent" {
						d, ok := agentDeduction[f.Severity]
						if !ok {
							warnings = append(warnings, fmt.Sprintf("%s/%s %s: confirmed agent finding has no severity; counted as medium", model, run, f.ID))
							d = 2
						}
						bugs -= d
					} else {
						bugs -= toolDeduction
					}
				case "pending", "":
					rs.Pending++
				}
			}
			rs.Bugs = math.Max(0, bugs)
		}
		key := model + "/" + run
		if r, ok := reviewOf[key]; ok {
			rs.Merge, rs.Reviewed = r.Approve+r.Understand, true
		}
		if rs.Pending > 0 {
			warnings = append(warnings, fmt.Sprintf("%s: %d finding(s) still pending a verdict", key, rs.Pending))
		}
		if !rs.Reviewed {
			warnings = append(warnings, fmt.Sprintf("%s: no blind review yet", key))
		}
		ms := models[model]
		if ms == nil {
			ms = &ModelScore{Model: model}
			models[model] = ms
		}
		ms.Runs = append(ms.Runs, rs)
	}

	var list []*ModelScore
	for _, ms := range models {
		n := float64(len(ms.Runs))
		var built, reviewed float64
		for _, r := range ms.Runs {
			ms.Works += r.Works / n
			ms.Bugs += r.Bugs / n
			ms.Tests += r.Tests / n
			ms.Cost += r.Stats.CostUSD / n
			ms.Seconds += r.Stats.Seconds / n
			ms.Calls += float64(r.Stats.ModelCalls) / n
			if r.Built {
				ms.StyleRaw += float64(r.StyleRaw)
				built++
			}
			if r.Reviewed {
				ms.Merge += r.Merge
				reviewed++
			}
			for _, f := range r.Confirmed {
				if f.Severity == "high" {
					ms.ConfirmedHigh++
				}
			}
		}
		if built > 0 {
			ms.StyleRaw /= built
			ms.Built = true
		}
		if reviewed > 0 {
			ms.Merge /= reviewed
		}
		list = append(list, ms)
	}

	// Readable Go: rank models by average style score (lower is better): 10/8/6/4/2.
	// Tied models share the better rank. A model with no building run gets 0.
	var ranked []*ModelScore
	for _, ms := range list {
		if ms.Built {
			ranked = append(ranked, ms)
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].StyleRaw < ranked[j].StyleRaw })
	for i, ms := range ranked {
		rank := i
		for rank > 0 && ranked[rank-1].StyleRaw == ms.StyleRaw {
			rank--
		}
		ms.Readable = math.Max(0, 10-2*float64(rank))
	}
	for _, ms := range list {
		ms.Total = ms.Works + ms.Bugs + ms.Readable + ms.Tests + ms.Merge
	}
	// Order: total, then fewer confirmed high-severity bugs, then lower cost.
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if round1(a.Total) != round1(b.Total) {
			return a.Total > b.Total
		}
		if a.ConfirmedHigh != b.ConfirmedHigh {
			return a.ConfirmedHigh < b.ConfirmedHigh
		}
		return a.Cost < b.Cost
	})

	var md strings.Builder
	md.WriteString("# Scorecard\n\n")
	if len(warnings) > 0 {
		md.WriteString("**Provisional.**\n\n")
		for _, w := range warnings {
			md.WriteString("- " + w + "\n")
		}
		md.WriteString("\n")
	}
	md.WriteString("| # | Model | Works /40 | Bugs & security /20 | Readable /10 | Own tests /10 | Would I merge /20 | **Total /100** | Cost per run | Time per run | Model calls |\n")
	md.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
	for i, ms := range list {
		fmt.Fprintf(&md, "| %d | %s | %.1f | %.1f | %.0f | %.1f | %.1f | **%.1f** | $%.2f | %s | %.0f |\n",
			i+1, ms.Model, ms.Works, ms.Bugs, ms.Readable, ms.Tests, ms.Merge, ms.Total, ms.Cost, dur(ms.Seconds), ms.Calls)
	}
	md.WriteString("\nScores are the average of the runs. Readable Go is a rank across models. Cost and time are shown, not scored.\n\n## Runs\n\n")
	md.WriteString("| Run | Builds | Hidden tests | Works | Confirmed findings | Bugs | Style raw | Own tests | Coverage | Merge | Cost | Time | Calls | Said DONE |\n")
	md.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, ms := range list {
		sort.Slice(ms.Runs, func(i, j int) bool { return ms.Runs[i].Run < ms.Runs[j].Run })
		for _, r := range ms.Runs {
			merge := "–"
			if r.Reviewed {
				merge = fmt.Sprintf("%.0f", r.Merge)
			}
			fmt.Fprintf(&md, "| %s/%s | %v | %d/%d | %.1f | %d | %.1f | %d | %s | %.1f%% | %s | $%.2f | %s | %d | %v |\n",
				r.Model, r.Run, yes(r.Built), r.HiddenPassed, r.HiddenTotal, r.Works, len(r.Confirmed), r.Bugs, r.StyleRaw,
				yes(r.OwnTestsPassed), r.Coverage, merge, r.Stats.CostUSD, dur(r.Stats.Seconds), r.Stats.ModelCalls, yes(r.Stats.Finished))
		}
	}
	if err := os.WriteFile(outMD, []byte(md.String()), 0o644); err != nil {
		return err
	}
	fmt.Print(md.String())
	return writeJSON(strings.TrimSuffix(outMD, filepath.Ext(outMD))+".json", list)
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func dur(sec float64) string {
	if sec == 0 {
		return "–"
	}
	return fmt.Sprintf("%dm%02ds", int(sec)/60, int(sec)%60)
}
