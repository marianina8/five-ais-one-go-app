package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

const transcriptResultBytes = 4000 // of each tool result; the model saw up to --max-output

// Transcript writes the run as Markdown, one step at a time, so a run that crashes still
// leaves everything up to the crash. Everything written goes through a Redactor first.
type Transcript struct {
	w       io.Writer
	redact  *Redactor
	started time.Time
}

func NewTranscript(w io.Writer, redact *Redactor, header string) *Transcript {
	t := &Transcript{w: w, redact: redact, started: time.Now()}
	t.write(header + "\n")
	return t
}

func (t *Transcript) write(s string) {
	_, _ = io.WriteString(t.w, t.redact.Clean(s))
	if f, ok := t.w.(*os.File); ok {
		_ = f.Sync()
	}
}

func (t *Transcript) User(task string) {
	t.write("## Prompt\n\n" + quote(task) + "\n")
}

func (t *Transcript) Reply(step int, resp Response, timeLeft time.Duration) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Step %d · %s in · %d tokens in, %d out · stop: %s · %s left\n\n",
		step, time.Since(t.started).Round(time.Second), resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.StopReason, timeLeft)
	if text := strings.TrimSpace(resp.Message.Text); text != "" {
		b.WriteString(quote(text) + "\n")
	}
	for _, call := range resp.Message.ToolCalls {
		b.WriteString(describeCall(call))
	}
	t.write(b.String())
}

func (t *Transcript) ToolResults(reply Message) {
	var b strings.Builder
	for _, result := range reply.ToolResults {
		status := "result"
		if result.IsError {
			status = "error"
		}
		fmt.Fprintf(&b, "<details><summary>%s %s (%d bytes)</summary>\n\n```text\n%s\n```\n</details>\n\n",
			result.Name, status, len(result.Output), fence(truncate(result.Output, transcriptResultBytes)))
	}
	b.WriteString("_" + reply.Text + "_\n\n")
	t.write(b.String())
}

func (t *Transcript) Stopped(step int, result Result) {
	line := fmt.Sprintf("## Stopped: %s after %d model calls and %s\n", result.StoppedBy, step, time.Since(t.started).Round(time.Second))
	if result.Error != "" {
		line += "\n```text\n" + fence(result.Error) + "\n```\n"
	}
	t.write(line)
}

// describeCall shows a tool call. A written file is shown in full, folded, since it's the
// best record of how the code evolved.
func describeCall(call ToolCall) string {
	var args toolArgs
	_ = json.Unmarshal(call.Input, &args)
	if call.Name == "write_file" {
		return fmt.Sprintf("<details><summary>→ write_file <code>%s</code> (%d bytes)</summary>\n\n```%s\n%s\n```\n</details>\n\n",
			args.Path, len(args.Content), codeLanguage(args.Path), fence(args.Content))
	}
	return fmt.Sprintf("→ `%s` `%s`\n\n", call.Name, string(call.Input))
}

func codeLanguage(path string) string {
	switch {
	case strings.HasSuffix(path, ".go"), strings.HasSuffix(path, "go.mod"):
		return "go"
	case strings.HasSuffix(path, ".json"):
		return "json"
	case strings.HasSuffix(path, ".md"):
		return "markdown"
	}
	return "text"
}

// fence keeps content from closing the Markdown code block it sits in.
func fence(s string) string { return strings.ReplaceAll(s, "```", "`\u200b``") }

func quote(s string) string { return "> " + strings.ReplaceAll(s, "\n", "\n> ") + "\n" }

// Redactor removes secrets from anything the harness writes: API keys and AWS credentials
// by value, and 12-digit AWS account IDs (they show up inside ARNs in AWS error messages).
type Redactor struct {
	secrets []string
}

var awsAccountID = regexp.MustCompile(`\b\d{12}\b`)

func NewRedactor(secrets ...string) *Redactor {
	r := &Redactor{}
	for _, s := range secrets {
		if len(s) >= 8 {
			r.secrets = append(r.secrets, s)
		}
	}
	return r
}

func (r *Redactor) Clean(s string) string {
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, "[redacted]")
	}
	return awsAccountID.ReplaceAllString(s, "[aws-account]")
}

// redactingWriter cleans everything written through it (the progress log).
type redactingWriter struct {
	w      io.Writer
	redact *Redactor
}

func (rw redactingWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(rw.w, rw.redact.Clean(string(p))); err != nil {
		return 0, err
	}
	return len(p), nil
}
