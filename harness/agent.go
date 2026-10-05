package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// Agent is the loop every contestant runs in: ask the model, run the tools it asks for,
// send the results back, and repeat until it answers without a tool call or runs out of
// model calls or time.
type Agent struct {
	Model      Model
	Tools      *Toolbox
	System     string
	MaxSteps   int       // model calls
	Deadline   time.Time // wall-clock limit for the whole run
	Log        io.Writer // progress for the CI log
	Transcript *Transcript
}

// Stop reasons.
const (
	StoppedDone  = "done"  // the model answered without calling a tool
	StoppedSteps = "steps" // it used every model call
	StoppedTime  = "time"  // the time limit hit
	StoppedError = "error" // the vendor's API kept failing
)

// Result is what happened in one run.
type Result struct {
	Steps       int            `json:"model_calls"`
	ToolCalls   int            `json:"tool_calls"`
	ToolsByName map[string]int `json:"tool_calls_by_name"`
	Usage       Usage          `json:"usage"`
	StoppedBy   string         `json:"stopped_by"`
	Error       string         `json:"error,omitempty"`
	FinalText   string         `json:"-"`
}

func (a *Agent) Run(ctx context.Context, task string) Result {
	ctx, cancel := context.WithDeadline(ctx, a.Deadline)
	defer cancel()

	result := Result{ToolsByName: map[string]int{}}
	messages := []Message{{Role: "user", Text: task}}
	a.Transcript.User(task)
	tools := a.Tools.Tools()

	for step := 1; step <= a.MaxSteps; step++ {
		resp, err := a.Model.Chat(ctx, a.System, messages, tools)
		result.Usage.Add(resp.Usage)
		if err != nil {
			result.Steps = step
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result.StoppedBy = StoppedTime
			} else {
				result.StoppedBy, result.Error = StoppedError, err.Error()
			}
			a.Transcript.Stopped(step, result)
			return result
		}
		result.Steps = step
		messages = append(messages, resp.Message)
		a.Transcript.Reply(step, resp, a.timeLeft())
		a.logf("step %d: %d tool call(s), stop %s, %s left", step, len(resp.Message.ToolCalls), resp.StopReason, a.timeLeft())

		if len(resp.Message.ToolCalls) == 0 {
			result.StoppedBy, result.FinalText = StoppedDone, resp.Message.Text
			a.Transcript.Stopped(step, result)
			return result
		}
		if step == a.MaxSteps || ctx.Err() != nil {
			break // no budget left to send results back
		}

		reply := Message{Role: "user"}
		for _, call := range resp.Message.ToolCalls {
			toolResult := a.Tools.Run(ctx, call)
			reply.ToolResults = append(reply.ToolResults, toolResult)
			result.ToolCalls++
			result.ToolsByName[call.Name]++
			a.logf("  %s %s", call.Name, firstLine(toolResult.Output))
		}
		reply.Text = budgetNote(a.MaxSteps-step, a.timeLeft())
		a.Transcript.ToolResults(reply)
		messages = append(messages, reply)
	}

	result.StoppedBy = StoppedSteps
	if ctx.Err() != nil {
		result.StoppedBy = StoppedTime
	}
	a.Transcript.Stopped(result.Steps, result)
	return result
}

// logf writes a progress line. The log is best effort: a failed write mustn't stop a run.
func (a *Agent) logf(format string, args ...any) {
	_, _ = fmt.Fprintf(a.Log, format+"\n", args...)
}

func (a *Agent) timeLeft() time.Duration {
	return max(0, time.Until(a.Deadline)).Round(time.Second)
}

// budgetNote goes with every batch of tool results, so each model knows its budget.
func budgetNote(callsLeft int, timeLeft time.Duration) string {
	if callsLeft == 1 {
		return fmt.Sprintf("Budget: this is your last model call (%s left). Tool calls in your next reply won't run: reply with DONE and your summary.", timeLeft)
	}
	return fmt.Sprintf("Budget: %d model calls and %s left.", callsLeft, timeLeft)
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}
