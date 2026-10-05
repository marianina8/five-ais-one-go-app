package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// scriptedModel replays a fixed list of replies, like a model that knows what to do.
type scriptedModel struct {
	replies []Message
	calls   int
}

func (m *scriptedModel) Chat(context.Context, string, []Message, []Tool) (Response, error) {
	if m.calls >= len(m.replies) {
		return Response{}, os.ErrDeadlineExceeded
	}
	reply := m.replies[m.calls]
	m.calls++
	return Response{Message: reply, Usage: Usage{InputTokens: 1000, OutputTokens: 100}, StopReason: "test"}, nil
}

func call(id, name string, args map[string]any) ToolCall {
	input, _ := json.Marshal(args)
	return ToolCall{ID: id, Name: name, Input: input}
}

func newTestAgent(t *testing.T, model Model, maxSteps int, deadline time.Time) (*Agent, string, *bytes.Buffer) {
	t.Helper()
	workspace := t.TempDir()
	tools, err := NewToolbox(workspace, &LocalSandbox{Timeout: 2 * time.Minute}, 20000)
	if err != nil {
		t.Fatal(err)
	}
	var transcript bytes.Buffer
	return &Agent{
		Model: model, Tools: tools, System: systemPrompt, MaxSteps: maxSteps, Deadline: deadline,
		Log: &bytes.Buffer{}, Transcript: NewTranscript(&transcript, NewRedactor(), "# test"),
	}, workspace, &transcript
}

// TestAgentBuildsTheReference has a scripted model write the reference implementation
// through the tools, build and vet it, and finish: the whole loop, end to end.
func TestAgentBuildsTheReference(t *testing.T) {
	writes := referenceWrites(t)
	model := &scriptedModel{replies: []Message{
		{Role: "assistant", Text: "Writing the files.", ToolCalls: writes},
		{Role: "assistant", ToolCalls: []ToolCall{call("b", "go_build", nil), call("v", "go_vet", map[string]any{"packages": "./..."})}},
		{Role: "assistant", ToolCalls: []ToolCall{call("l", "list_files", map[string]any{"path": "."}), call("r", "read_file", map[string]any{"path": "go.mod"})}},
		{Role: "assistant", Text: "DONE: a URL shortener."},
	}}
	agent, workspace, transcript := newTestAgent(t, model, 10, time.Now().Add(5*time.Minute))
	result := agent.Run(context.Background(), "build it")

	if result.StoppedBy != StoppedDone || result.Steps != 4 || result.ToolCalls != 9 {
		t.Fatalf("result %+v", result)
	}
	if result.ToolsByName["write_file"] != 5 || result.Usage.InputTokens != 4000 {
		t.Errorf("counts %+v", result)
	}
	if _, err := os.Stat(filepath.Join(workspace, "store.go")); err != nil {
		t.Errorf("file not written: %v", err)
	}
	text := transcript.String()
	for _, want := range []string{"## Step 1", "write_file <code>store.go</code>", "exit code 0", "## Stopped: done after 4 model calls", "Budget: 9 model calls"} {
		if !strings.Contains(text, want) {
			t.Errorf("transcript is missing %q", want)
		}
	}
}

// referenceWrites is one write_file call per file of the reference implementation.
func referenceWrites(t *testing.T) []ToolCall {
	t.Helper()
	var writes []ToolCall
	for _, name := range []string{"go.mod", "main.go", "store.go", "api.go", "validate.go"} {
		source, err := os.ReadFile(filepath.Join("..", "reference", name)) // #nosec G304 -- the repo's own reference files
		if err != nil {
			t.Fatal(err)
		}
		writes = append(writes, call("w-"+name, "write_file", map[string]any{"path": name, "content": string(source)}))
	}
	return writes
}

func TestAgentStopsAtStepLimit(t *testing.T) {
	looping := &scriptedModel{}
	for range 5 {
		looping.replies = append(looping.replies, Message{Role: "assistant", ToolCalls: []ToolCall{call("l", "list_files", nil)}})
	}
	agent, _, transcript := newTestAgent(t, looping, 3, time.Now().Add(time.Minute))
	result := agent.Run(context.Background(), "build it")
	if result.StoppedBy != StoppedSteps || result.Steps != 3 || result.ToolCalls != 2 {
		t.Fatalf("result %+v: the last call's tools must not run", result)
	}
	if !strings.Contains(transcript.String(), "this is your last model call") {
		t.Error("the model should be told when its next call is the last")
	}
}

func TestAgentStopsAtTimeLimit(t *testing.T) {
	agent, _, _ := newTestAgent(t, &scriptedModel{}, 10, time.Now().Add(-time.Second))
	if result := agent.Run(context.Background(), "build it"); result.StoppedBy != StoppedTime {
		t.Fatalf("result %+v", result)
	}
}

func TestToolsStayInsideTheWorkspace(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("hidden"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent, workspace, _ := newTestAgent(t, nil, 1, time.Now())
	if err := os.Symlink(outside, filepath.Join(workspace, "link")); err != nil {
		t.Fatal(err)
	}
	cases := []ToolCall{
		call("1", "read_file", map[string]any{"path": "../secret.txt"}),
		call("2", "read_file", map[string]any{"path": filepath.Join(outside, "secret.txt")}),
		call("3", "read_file", map[string]any{"path": "link/secret.txt"}),
		call("4", "write_file", map[string]any{"path": "link/new.go", "content": "x"}),
		call("5", "list_files", map[string]any{"path": "link"}),
		call("6", "go_test", map[string]any{"packages": "-exec=sh"}),
		call("7", "go_build", map[string]any{"packages": "golang.org/x/tools@latest"}),
		call("8", "go_vet", map[string]any{"packages": "./../.."}),
		call("9", "write_file", map[string]any{"path": "big.go", "content": strings.Repeat("x", maxWriteBytes+1)}),
		call("10", "write_file", map[string]any{"path": ".", "content": "x"}),
		call("11", "delete_file", map[string]any{"path": "x"}),
	}
	for _, c := range cases {
		if result := agent.Tools.Run(context.Background(), c); !result.IsError || strings.Contains(result.Output, "hidden") {
			t.Errorf("%s %s should be refused, got %q", c.Name, c.Input, result.Output)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "new.go")); err == nil {
		t.Error("write_file wrote outside the workspace")
	}
}

// TestGoCommandsGetNoSecrets checks that the contestant's code, run by go test, can't see
// API keys or AWS credentials from the harness's environment.
func TestGoCommandsGetNoSecrets(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-should-not-leak")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-should-not-leak")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "oidc-should-not-leak")
	agent, workspace, _ := newTestAgent(t, nil, 1, time.Now())
	files := map[string]string{
		"go.mod":      "module probe\n\ngo 1.24\n",
		"env_test.go": "package probe\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestEnv(t *testing.T) {\n\tfor _, kv := range os.Environ() {\n\t\tt.Log(kv)\n\t}\n}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result := agent.Tools.Run(context.Background(), call("t", "go_test", map[string]any{"verbose": true}))
	if !strings.Contains(result.Output, "GOPROXY=off") {
		t.Fatalf("probe didn't run: %s", result.Output)
	}
	for _, secret := range []string{"should-not-leak", "ANTHROPIC", "AWS_", "ACTIONS_"} {
		if strings.Contains(result.Output, secret) {
			t.Errorf("go test saw %q:\n%s", secret, result.Output)
		}
	}
}

func TestTakeAPIKeysClearsTheEnvironment(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-proj-123456789")
	keys := takeAPIKeys()
	if keys.openai != "sk-proj-123456789" || os.Getenv("OPENAI_API_KEY") != "" {
		t.Fatalf("keys %+v, env still %q", keys, os.Getenv("OPENAI_API_KEY"))
	}
}

func TestRedactor(t *testing.T) {
	r := NewRedactor("sk-ant-abcdefgh", "", "short")
	got := r.Clean("key sk-ant-abcdefgh, role arn:aws:iam::123456789012:role/x, port 8080, short")
	if strings.Contains(got, "sk-ant") || strings.Contains(got, "123456789012") {
		t.Errorf("not redacted: %s", got)
	}
	if !strings.Contains(got, "8080") || !strings.Contains(got, "short") {
		t.Errorf("redacted too much: %s", got)
	}
}

func TestPrices(t *testing.T) {
	for _, model := range []string{"claude-sonnet-5-5", "claude-opus-5-5", "gpt-6.1-sol", "gemini-3.1-pro-preview", "deepseek.v3.2"} {
		price, err := priceOf(model)
		if err != nil || price.Input <= 0 || price.Output <= 0 {
			t.Errorf("%s: %+v %v", model, price, err)
		}
	}
	if got := cost(Usage{InputTokens: 1_000_000, OutputTokens: 100_000}, Price{Input: 2, Output: 10}); got != 3 {
		t.Errorf("cost = %v, want 3", got)
	}
}

func TestWorkspaceMustStartEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := requireEmptyDir(dir); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "leftover.go"), nil, 0o600)
	if err := requireEmptyDir(dir); err == nil {
		t.Fatal("a non-empty workspace must be refused")
	}
}

// TestRunWritesTranscriptAndStats runs the whole program against a fake Anthropic API: the
// model writes a file, then finishes. The key must not appear in anything written.
func TestRunWritesTranscriptAndStats(t *testing.T) {
	const key = "sk-ant-run-test-key-123"
	api := &fakeAPI{t: t, path: "/v1/messages", header: "x-api-key", key: key, replies: []string{
		`{"content":[{"type":"tool_use","id":"tu_1","name":"write_file","input":{"path":"go.mod","content":"module shortener\n\ngo 1.24\n"}}],"stop_reason":"tool_use","usage":{"input_tokens":1000000,"output_tokens":100000}}`,
		`{"content":[{"type":"text","text":"DONE: wrote go.mod"}],"stop_reason":"end_turn","usage":{"input_tokens":0,"output_tokens":0}}`,
	}}
	t.Setenv("ANTHROPIC_BASE_URL", serve(t, api))
	t.Setenv("ANTHROPIC_API_KEY", key)
	dir := t.TempDir()
	c := runConfig{
		provider: "anthropic", model: "claude-sonnet-5-5", label: "test-run",
		specPath: filepath.Join("..", "spec", "SPEC.md"), workspace: filepath.Join(dir, "workspace"), outDir: filepath.Join(dir, "out"),
		maxSteps: 5, minutes: 1, maxTokens: 16000, maxOut: 20000, sandbox: "local", toolTimeout: time.Minute,
	}
	if err := run(c); err != nil {
		t.Fatal(err)
	}

	stats, statsJSON, transcript := readRunOutput(t, filepath.Join(dir, "out"))
	want := Stats{Finished: true, ModelCalls: 2, ToolCalls: 1, CostUSD: 3} // $3 at Sonnet prices
	if got := (Stats{Finished: stats.Finished, ModelCalls: stats.ModelCalls, ToolCalls: stats.ToolCalls, CostUSD: stats.CostUSD}); !reflect.DeepEqual(got, want) {
		t.Errorf("stats %+v, want %+v", got, want)
	}
	if !strings.Contains(transcript, "## Prompt") || !strings.Contains(transcript, "Build a URL shortener in Go") {
		t.Error("the transcript should start with the prompt, including the spec")
	}
	if strings.Contains(statsJSON+transcript, key) {
		t.Error("the API key leaked into an output file")
	}
	_, statErr := os.Stat(filepath.Join(dir, "workspace", "go.mod"))
	if statErr != nil || os.Getenv("ANTHROPIC_API_KEY") != "" {
		t.Errorf("the model's file should exist (%v) and the key be gone from the environment", statErr)
	}
}

func readRunOutput(t *testing.T, outDir string) (stats Stats, statsJSON, transcript string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(outDir, "stats.json")) // #nosec G304 -- test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &stats); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(outDir, "transcript.md")) // #nosec G304 -- as above
	if err != nil {
		t.Fatal(err)
	}
	return stats, string(data), string(text)
}
