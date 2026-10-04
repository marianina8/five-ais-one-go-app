package main

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"slices"
)

// Results layout, one folder per run:
//
//	results/<model>/<run>/measure.json   machine measurements (score measure)
//	results/<model>/<run>/findings.json  tool and review-agent findings with a person's verdicts
//	results/<model>/<run>/stats.json     harness log: model calls, tokens, cost, time
//	results/blind/map.json               {"A": "<model>/<run>", ...}, sealed until the reveal
//	results/blind/reviews.json           {"A": {"approve": 0-10, "understand": 0-10, "notes": ""}}

const maxBugsPoints = 20.0

// Points a confirmed finding costs. The review agent rates its findings' severity;
// a confirmed tool finding always costs toolFindingCost.
var agentFindingCost = map[string]float64{"high": 4, "medium": 2, "low": 1}

const toolFindingCost = 2.0

// Stats is what the harness logged about one run.
type Stats struct {
	ModelCalls   int     `json:"model_calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Seconds      float64 `json:"seconds"`
	Finished     bool    `json:"finished"` // the model said DONE, rather than running out of steps or time
}

// Review is one blind "Would I merge it?" score.
type Review struct {
	Approve    float64 `json:"approve"`    // would I approve it as a pull request, 0-10
	Understand float64 `json:"understand"` // could a teammate understand it in five minutes, 0-10
	Notes      string  `json:"notes"`
}

// RunScore is one run's points and the facts behind them.
type RunScore struct {
	Model, Run     string
	Built          bool
	Works          float64
	Bugs           float64
	OwnTests       float64
	Merge          float64
	Reviewed       bool
	StyleScore     int
	StyleMeasured  bool
	Confirmed      []Finding
	Pending        int
	Stats          Stats
	HiddenPassed   int
	HiddenTotal    int
	Coverage       float64
	OwnTestsPassed bool
}

// ModelScore is a model's points: the average over its runs, plus Readable, which is
// a rank across models.
type ModelScore struct {
	Model         string
	Runs          []*RunScore
	Works         float64
	Bugs          float64
	Readable      float64
	OwnTests      float64
	Merge         float64
	Total         float64
	StyleScore    float64 // average over the runs that built and could be measured
	AnyBuilt      bool    // at least one run built and its style was measured
	ConfirmedHigh int
	CostUSD       float64
	Seconds       float64
	ModelCalls    float64
}

// tally scores every run under resultsDir and writes the scorecard.
func tally(resultsDir, scorecardFile string) error {
	runs, warnings, err := loadRuns(resultsDir)
	if err != nil {
		return err
	}
	models := combineRuns(runs)
	rankReadability(models)
	sortModels(models)
	return writeScorecard(scorecardFile, models, warnings)
}

// loadRuns reads every run folder and scores what can be scored per run.
func loadRuns(resultsDir string) ([]*RunScore, []string, error) {
	measureFiles, err := filepath.Glob(filepath.Join(resultsDir, "*", "*", "measure.json"))
	if err != nil {
		return nil, nil, err
	}
	if len(measureFiles) == 0 {
		return nil, nil, fmt.Errorf("no %s/<model>/<run>/measure.json files", resultsDir)
	}
	reviews, err := loadBlindReviews(resultsDir)
	if err != nil {
		return nil, nil, err
	}

	var runs []*RunScore
	var warnings []string
	for _, measureFile := range measureFiles {
		runDir := filepath.Dir(measureFile)
		run := &RunScore{Run: filepath.Base(runDir), Model: filepath.Base(filepath.Dir(runDir))}
		name := run.Model + "/" + run.Run

		var m Measurement
		if err := readJSON(measureFile, &m); err != nil {
			return nil, nil, err
		}
		findings, runWarnings, err := loadRunFiles(runDir, name, m, &run.Stats)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, runWarnings...)
		scoreRun(run, m, findings, &warnings)
		if review, ok := reviews[name]; ok {
			run.Merge, run.Reviewed = review.Approve+review.Understand, true
		} else {
			warnings = append(warnings, fmt.Sprintf("%s: no blind review yet", name))
		}
		runs = append(runs, run)
	}
	return runs, warnings, nil
}

// loadRunFiles reads a run's findings.json (the verdicts) and stats.json. A missing file is
// a warning; a file that exists but can't be read is an error, so a typo in hand-edited
// verdicts stops the tally instead of quietly dropping them.
func loadRunFiles(runDir, name string, m Measurement, stats *Stats) ([]Finding, []string, error) {
	var warnings []string
	var findings []Finding
	switch err := readJSON(filepath.Join(runDir, "findings.json"), &findings); {
	case errors.Is(err, fs.ErrNotExist):
		findings = m.Findings
		warnings = append(warnings, fmt.Sprintf("%s: no findings.json, using the unreviewed tool findings", name))
	case err != nil:
		return nil, nil, err
	}
	switch err := readJSON(filepath.Join(runDir, "stats.json"), stats); {
	case errors.Is(err, fs.ErrNotExist):
		warnings = append(warnings, fmt.Sprintf("%s: no stats.json (cost and time unknown)", name))
	case err != nil:
		return nil, nil, err
	}
	if m.Build.OK && !m.Style.Measured {
		warnings = append(warnings, fmt.Sprintf("%s: style could not be measured; left out of the Readable Go ranking", name))
	}
	return findings, warnings, nil
}

// loadBlindReviews returns each run's blind review, keyed by "<model>/<run>".
// Before the blind review exists, it returns an empty map.
func loadBlindReviews(resultsDir string) (map[string]Review, error) {
	byRun := map[string]Review{}
	letterToRun := map[string]string{}
	reviewsByLetter := map[string]Review{}
	mapFile := filepath.Join(resultsDir, "blind", "map.json")
	switch err := readJSON(mapFile, &letterToRun); {
	case errors.Is(err, fs.ErrNotExist):
		return byRun, nil // no blind review yet
	case err != nil:
		return nil, err
	}
	if err := readJSON(filepath.Join(resultsDir, "blind", "reviews.json"), &reviewsByLetter); err != nil {
		return nil, err
	}
	for letter, run := range letterToRun {
		review, ok := reviewsByLetter[letter]
		if ok && (review.Approve > 0 || review.Understand > 0 || review.Notes != "") {
			byRun[run] = review
		}
	}
	return byRun, nil
}

// scoreRun fills in the measured points and the confirmed-findings deduction.
func scoreRun(run *RunScore, m Measurement, findings []Finding, warnings *[]string) {
	run.Built = m.Build.OK
	run.Coverage = m.OwnTests.Coverage
	run.OwnTestsPassed = m.OwnTests.Passed
	for _, counts := range m.Hidden.Categories {
		run.HiddenPassed += counts[0]
		run.HiddenTotal += counts[1]
	}
	if !run.Built {
		return // a project that doesn't build scores 0 in every measured category
	}
	run.Works = m.Hidden.Total
	run.OwnTests = m.OwnTests.Points
	run.StyleScore = m.Style.Score
	run.StyleMeasured = m.Style.Measured

	name := run.Model + "/" + run.Run
	bugs := maxBugsPoints
	for _, finding := range findings {
		switch finding.Verdict {
		case "confirmed":
			run.Confirmed = append(run.Confirmed, finding)
			bugs -= findingCost(finding, name, warnings)
		case "pending", "":
			run.Pending++
		case "rejected", "duplicate":
		default:
			*warnings = append(*warnings, fmt.Sprintf("%s %s: unknown verdict %q (treated as pending)", name, finding.ID, finding.Verdict))
			run.Pending++
		}
	}
	run.Bugs = math.Max(0, bugs)
	if run.Pending > 0 {
		*warnings = append(*warnings, fmt.Sprintf("%s: %d finding(s) still need a verdict", name, run.Pending))
	}
}

func findingCost(finding Finding, runName string, warnings *[]string) float64 {
	if finding.Source != "agent" {
		return toolFindingCost
	}
	cost, ok := agentFindingCost[finding.Severity]
	if !ok {
		*warnings = append(*warnings, fmt.Sprintf("%s %s: confirmed agent finding has no severity; counted as medium", runName, finding.ID))
		return agentFindingCost["medium"]
	}
	return cost
}

// combineRuns averages each model's runs.
func combineRuns(runs []*RunScore) []*ModelScore {
	byModel := map[string]*ModelScore{}
	var models []*ModelScore
	for _, run := range runs {
		model := byModel[run.Model]
		if model == nil {
			model = &ModelScore{Model: run.Model}
			byModel[run.Model] = model
			models = append(models, model)
		}
		model.Runs = append(model.Runs, run)
	}
	for _, model := range models {
		averageRuns(model)
	}
	return models
}

// averageRuns sets a model's points to the average of its runs. Style is averaged over the
// runs that built, and Merge over the runs reviewed so far.
func averageRuns(model *ModelScore) {
	count := float64(len(model.Runs))
	var built, reviewed float64
	for _, run := range model.Runs {
		model.Works += run.Works / count
		model.Bugs += run.Bugs / count
		model.OwnTests += run.OwnTests / count
		model.CostUSD += run.Stats.CostUSD / count
		model.Seconds += run.Stats.Seconds / count
		model.ModelCalls += float64(run.Stats.ModelCalls) / count
		if run.Built && run.StyleMeasured {
			model.StyleScore += float64(run.StyleScore)
			built++
		}
		if run.Reviewed {
			model.Merge += run.Merge
			reviewed++
		}
		for _, finding := range run.Confirmed {
			if finding.Severity == "high" {
				model.ConfirmedHigh++
			}
		}
	}
	if built > 0 {
		model.StyleScore /= built
		model.AnyBuilt = true
	}
	if reviewed > 0 {
		model.Merge /= reviewed
	}
	slices.SortFunc(model.Runs, func(a, b *RunScore) int { return cmp.Compare(a.Run, b.Run) })
}

// rankReadability gives Readable Go points by rank of the average style score (lower is
// better): 10, 8, 6, 4, 2. Tied models share the better rank; a model whose runs never
// built gets 0. It also fills in each model's total.
func rankReadability(models []*ModelScore) {
	var ranked []*ModelScore
	for _, model := range models {
		if model.AnyBuilt {
			ranked = append(ranked, model)
		}
	}
	slices.SortStableFunc(ranked, func(a, b *ModelScore) int { return cmp.Compare(a.StyleScore, b.StyleScore) })
	for i, model := range ranked {
		rank := i
		for rank > 0 && ranked[rank-1].StyleScore == model.StyleScore {
			rank--
		}
		model.Readable = math.Max(0, 10-2*float64(rank))
	}
	for _, model := range models {
		model.Total = model.Works + model.Bugs + model.Readable + model.OwnTests + model.Merge
	}
}

// sortModels orders by total, then fewer confirmed high-severity bugs, then lower cost.
func sortModels(models []*ModelScore) {
	slices.SortStableFunc(models, func(a, b *ModelScore) int {
		return cmp.Or(
			cmp.Compare(round1(b.Total), round1(a.Total)),
			cmp.Compare(a.ConfirmedHigh, b.ConfirmedHigh),
			cmp.Compare(a.CostUSD, b.CostUSD),
		)
	})
}
