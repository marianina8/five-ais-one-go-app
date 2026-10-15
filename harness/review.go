package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"time"
)

// The Ep04 bonus round: the Ep01 PR review agent, available to the contestant as a
// request_review tool. Its rules are the Ep01 agent's guidelines.md, copied unmodified from
// github.com/marianina8/pr-review-agent. It runs as a separate, read-only agent on the same
// model, so the contestant gets a second opinion from a fresh conversation, not from itself.

//go:embed review_guidelines.md
var reviewGuidelines string

// reviewNote tells the reviewer which of its Ep01 tools exist here. Ep01's agent could also
// write scratch tests; in the contest workspace it may only read and run the checks.
const reviewNote = "\n\n## In this review\n\nThe `write_scratch_test` tool and grep aren't available here. You can list and read files and run `go_vet` and `go_test`. Check suspicions by reading the code and running the existing tests."

const (
	reviewMaxSteps = 15
	reviewMaxTime  = 6 * time.Minute
)

// Reviewer runs the review agent and keeps a tally of what it cost, which counts toward the
// contestant's run.
type Reviewer struct {
	Model Model
	Tools *Toolbox // read-only view of the contestant's workspace
	Spec  string
	Calls int
	Usage Usage
}

func (r *Reviewer) Review(ctx context.Context) (string, error) {
	deadline := time.Now().Add(reviewMaxTime)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	agent := &Agent{
		Model:      r.Model,
		Tools:      r.Tools,
		System:     reviewGuidelines + reviewNote,
		MaxSteps:   reviewMaxSteps,
		Deadline:   deadline,
		Log:        io.Discard,
		Transcript: NewTranscript(io.Discard, NewRedactor(), ""),
	}
	task := "Review the Go code in this workspace. It is a pull request implementing this spec:\n\n" + r.Spec
	result := agent.Run(ctx, task)
	r.Calls++
	r.Usage.Add(result.Usage)
	if result.StoppedBy != StoppedDone || result.FinalText == "" {
		return "", fmt.Errorf("the review agent stopped (%s) without writing a review", result.StoppedBy)
	}
	return "Review from the PR review agent (a separate reviewer, same model):\n\n" + result.FinalText, nil
}
