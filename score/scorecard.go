package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeScorecard writes the scorecard as Markdown (and the same data as JSON next to it),
// and prints the Markdown.
func writeScorecard(file string, models []*ModelScore, warnings []string) error {
	var md strings.Builder
	md.WriteString("# Scorecard\n\n")
	if len(warnings) > 0 {
		md.WriteString("**Provisional:**\n\n")
		for _, warning := range warnings {
			md.WriteString("- " + warning + "\n")
		}
		md.WriteString("\n")
	}

	md.WriteString("| # | Model | Works /40 | Bugs & security /20 | Readable /10 | Own tests /10 | Would I merge /20 | **Total /100** | Cost per run | Time per run | Model calls |\n")
	md.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
	for i, model := range models {
		fmt.Fprintf(&md, "| %d | %s | %.1f | %.1f | %.0f | %.1f | %.1f | **%.1f** | $%.2f | %s | %.0f |\n",
			i+1, model.Model, model.Works, model.Bugs, model.Readable, model.OwnTests, model.Merge, model.Total,
			model.CostUSD, minutes(model.Seconds), model.ModelCalls)
	}
	md.WriteString("\nEach model's points are the average of its runs. Readable Go is a rank across models. Cost and time are shown, not scored.\n\n")

	md.WriteString("## Runs\n\n")
	md.WriteString("| Run | Builds | Hidden tests | Works | Confirmed findings | Bugs | Style score | Own tests pass | Coverage | Merge | Cost | Time | Model calls | Said DONE |\n")
	md.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, model := range models {
		for _, run := range model.Runs {
			merge := "–"
			if run.Reviewed {
				merge = fmt.Sprintf("%.0f", run.Merge)
			}
			fmt.Fprintf(&md, "| %s/%s | %s | %d/%d | %.1f | %d | %.1f | %d | %s | %.1f%% | %s | $%.2f | %s | %d | %s |\n",
				run.Model, run.Run, yesNo(run.Built), run.HiddenPassed, run.HiddenTotal, run.Works,
				len(run.Confirmed), run.Bugs, run.StyleScore, yesNo(run.OwnTestsPassed), run.Coverage, merge,
				run.Stats.CostUSD, minutes(run.Stats.Seconds), run.Stats.ModelCalls, yesNo(run.Stats.Finished))
		}
	}

	if err := os.WriteFile(file, []byte(md.String()), 0o600); err != nil {
		return err
	}
	fmt.Print(md.String())
	return writeJSON(strings.TrimSuffix(file, filepath.Ext(file))+".json", models)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// minutes formats seconds as "4m05s"; 0 (unknown) is "–".
func minutes(seconds float64) string {
	if seconds == 0 {
		return "–"
	}
	return fmt.Sprintf("%dm%02ds", int(seconds)/60, int(seconds)%60)
}
