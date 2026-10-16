// Command harness runs one contestant: it gives a model the spec, an empty workspace and
// the same six tools as every other model, and records what happens.
//
//	harness --provider anthropic --model claude-sonnet-5-5 --spec spec/SPEC.md \
//	        --workspace /tmp/run/workspace --out /tmp/run
//
// It writes <out>/transcript.md and <out>/stats.json. The workspace is left exactly as the
// model left it. API keys come from ANTHROPIC_API_KEY, OPENAI_API_KEY or GEMINI_API_KEY;
// Bedrock uses the usual AWS credentials.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

//go:embed system.md
var systemPrompt string

//go:embed prices.json
var pricesJSON []byte

// Price is a model's list price in US dollars per million tokens.
type Price struct {
	Input           float64 `json:"input"`
	Output          float64 `json:"output"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty"`
	Source          string  `json:"source"`
}

// Stats is the run's record. tally reads model_calls, cost_usd, seconds and finished.
type Stats struct {
	Label           string         `json:"label"`
	Provider        string         `json:"provider"`
	Model           string         `json:"model"`
	StartedAt       time.Time      `json:"started_at"`
	MaxSteps        int            `json:"max_steps"`
	Minutes         float64        `json:"minutes"`
	MaxOutputTokens int            `json:"max_output_tokens"`
	ModelCalls      int            `json:"model_calls"`
	ToolCalls       int            `json:"tool_calls"`
	ToolsByName     map[string]int `json:"tool_calls_by_name"`
	InputTokens     int            `json:"input_tokens"`
	OutputTokens    int            `json:"output_tokens"`
	CachedTokens    int            `json:"cached_tokens"`
	Price           Price          `json:"price"`
	CostUSD         float64        `json:"cost_usd"`
	Seconds         float64        `json:"seconds"`
	Finished        bool           `json:"finished"` // answered on its own, within budget
	StoppedBy       string         `json:"stopped_by"`
	Error           string         `json:"error,omitempty"`
	FinalText       string         `json:"final_text,omitempty"`
}

// runConfig is everything the flags set.
type runConfig struct {
	provider, model, label      string
	specPath, workspace, outDir string
	maxSteps, maxTokens, maxOut int
	minutes                     float64
	sandbox, image, region      string
	toolTimeout                 time.Duration
}

func main() {
	if err := run(parseFlags()); err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		os.Exit(1)
	}
}

func parseFlags() runConfig {
	var c runConfig
	flag.StringVar(&c.provider, "provider", "", "anthropic, openai, gemini or bedrock")
	flag.StringVar(&c.model, "model", "", "the vendor's model ID, e.g. claude-sonnet-5-5")
	flag.StringVar(&c.label, "label", "", "name for this run (default: the model)")
	flag.StringVar(&c.specPath, "spec", "spec/SPEC.md", "the spec every model gets")
	flag.StringVar(&c.workspace, "workspace", "", "an empty folder for the model to work in")
	flag.StringVar(&c.outDir, "out", "", "folder for transcript.md and stats.json")
	flag.IntVar(&c.maxSteps, "max-steps", 40, "model calls")
	flag.Float64Var(&c.minutes, "minutes", 20, "wall-clock limit for the whole run")
	flag.IntVar(&c.maxTokens, "max-tokens", 16000, "output tokens per model call (lowered to the model's own maximum)")
	flag.IntVar(&c.maxOut, "max-output", 20000, "bytes of one tool result the model sees")
	flag.StringVar(&c.sandbox, "sandbox", "docker", "docker (no network; used in the contest) or local")
	flag.StringVar(&c.image, "image", "golang:1.24", "docker image for go commands")
	flag.DurationVar(&c.toolTimeout, "tool-timeout", 3*time.Minute, "limit for one go command")
	flag.StringVar(&c.region, "region", "us-west-2", "bedrock: AWS region")
	flag.Parse()
	if c.label == "" {
		c.label = c.model
	}
	return c
}

func run(c runConfig) error {
	if c.provider == "" || c.model == "" || c.workspace == "" || c.outDir == "" {
		return fmt.Errorf("--provider, --model, --workspace and --out are required")
	}
	spec, price, err := prepare(&c)
	if err != nil {
		return err
	}

	transcriptFile, err := os.Create(filepath.Join(c.outDir, "transcript.md"))
	if err != nil {
		return err
	}
	agent, redact, err := newAgent(c, transcriptFile)
	if err != nil {
		_ = transcriptFile.Close()
		return err
	}
	started := time.Now()
	agent.Deadline = started.Add(time.Duration(c.minutes * float64(time.Minute)))
	task := fmt.Sprintf("Your workspace is empty. Build the service described in this spec.\n\n%s\n\nBudget: %d model calls and %.0f minutes.", spec, c.maxSteps, c.minutes)
	result := agent.Run(context.Background(), task)
	if err := transcriptFile.Close(); err != nil {
		return err
	}

	stats := newStats(c, price, result, started, redact)
	agent.logf("%s: stopped by %s after %d model calls, %d tool calls, %.0fs, $%.2f",
		c.label, stats.StoppedBy, stats.ModelCalls, stats.ToolCalls, stats.Seconds, stats.CostUSD)
	return writeJSON(filepath.Join(c.outDir, "stats.json"), stats)
}

// prepare reads the spec and the model's price, checks the workspace is empty, creates the
// output folder, and lowers the per-call output limit to the model's own maximum.
func prepare(c *runConfig) (spec []byte, price Price, err error) {
	if spec, err = os.ReadFile(c.specPath); err != nil {
		return nil, Price{}, err
	}
	if err := requireEmptyDir(c.workspace); err != nil {
		return nil, Price{}, err
	}
	if err := os.MkdirAll(c.outDir, 0o750); err != nil {
		return nil, Price{}, err
	}
	if price, err = priceOf(c.model); err != nil {
		return nil, Price{}, err
	}
	if price.MaxOutputTokens > 0 {
		c.maxTokens = min(c.maxTokens, price.MaxOutputTokens)
	}
	return spec, price, nil
}

// newAgent connects to the model and sets up the tools, log and transcript. API keys are
// taken out of the environment here, before anything else can run.
func newAgent(c runConfig, transcript *os.File) (*Agent, *Redactor, error) {
	keys := takeAPIKeys()
	redact := NewRedactor(append(keys.all(), os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_SESSION_TOKEN"))...)
	llm, err := newModel(context.Background(), c.provider, c.model, c.maxTokens, c.region, keys)
	if err != nil {
		return nil, nil, err
	}
	sandbox, err := newSandbox(c.sandbox, c.image, c.toolTimeout)
	if err != nil {
		return nil, nil, err
	}
	tools, err := NewToolbox(c.workspace, sandbox, c.maxOut)
	if err != nil {
		return nil, nil, err
	}
	header := fmt.Sprintf("# %s\n\n`%s` via %s · up to %d model calls, %.0f minutes, %d output tokens per call · tools: list_files, read_file, write_file, go_build, go_test, go_vet\n",
		c.label, c.model, c.provider, c.maxSteps, c.minutes, c.maxTokens)
	return &Agent{
		Model:      llm,
		Tools:      tools,
		System:     systemPrompt,
		MaxSteps:   c.maxSteps,
		Log:        redactingWriter{w: os.Stderr, redact: redact},
		Transcript: NewTranscript(transcript, redact, header),
	}, redact, nil
}

func newStats(c runConfig, price Price, result Result, started time.Time, redact *Redactor) Stats {
	return Stats{
		Label: c.label, Provider: c.provider, Model: c.model, StartedAt: started.UTC(),
		MaxSteps: c.maxSteps, Minutes: c.minutes, MaxOutputTokens: c.maxTokens,
		ModelCalls: result.Steps, ToolCalls: result.ToolCalls, ToolsByName: result.ToolsByName,
		InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens, CachedTokens: result.Usage.CachedTokens,
		Price:     price,
		CostUSD:   cost(result.Usage, price),
		Seconds:   time.Since(started).Round(time.Second).Seconds(),
		Finished:  result.StoppedBy == StoppedDone,
		StoppedBy: result.StoppedBy,
		Error:     redact.Clean(result.Error),
		FinalText: redact.Clean(result.FinalText),
	}
}

// apiKeys holds the vendors' keys. takeAPIKeys removes them from the environment, so no
// child process could inherit them even by mistake.
type apiKeys struct{ anthropic, openai, gemini string }

func takeAPIKeys() apiKeys {
	keys := apiKeys{os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("OPENAI_API_KEY"), os.Getenv("GEMINI_API_KEY")}
	for _, name := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY"} {
		_ = os.Unsetenv(name)
	}
	return keys
}

func (k apiKeys) all() []string { return []string{k.anthropic, k.openai, k.gemini} }

func newModel(ctx context.Context, provider, model string, maxTokens int, region string, keys apiKeys) (Model, error) {
	need := func(key, name string) error {
		if key == "" {
			return fmt.Errorf("--provider %s needs %s", provider, name)
		}
		return nil
	}
	switch provider {
	case "anthropic":
		// ANTHROPIC_BASE_URL is only for tests (a fake API server).
		return NewAnthropic(model, maxTokens, keys.anthropic, os.Getenv("ANTHROPIC_BASE_URL")), need(keys.anthropic, "ANTHROPIC_API_KEY")
	case "openai":
		return NewOpenAI(model, maxTokens, keys.openai, ""), need(keys.openai, "OPENAI_API_KEY")
	case "gemini":
		return NewGemini(model, maxTokens, keys.gemini, ""), need(keys.gemini, "GEMINI_API_KEY")
	case "bedrock":
		return NewBedrock(ctx, region, model, maxTokens)
	}
	return nil, fmt.Errorf("unknown --provider %q (anthropic, openai, gemini or bedrock)", provider)
}

func newSandbox(kind, image string, timeout time.Duration) (Sandbox, error) {
	switch kind {
	case "docker":
		cache := filepath.Join(os.TempDir(), "harness-gocache")
		if err := os.MkdirAll(cache, 0o750); err != nil {
			return nil, err
		}
		return &DockerSandbox{Image: image, CacheDir: cache, Timeout: timeout}, nil
	case "local":
		return &LocalSandbox{Timeout: timeout}, nil
	}
	return nil, fmt.Errorf("unknown --sandbox %q (docker or local)", kind)
}

func priceOf(model string) (Price, error) {
	var prices map[string]json.RawMessage
	if err := json.Unmarshal(pricesJSON, &prices); err != nil {
		return Price{}, err
	}
	raw, ok := prices[model]
	if !ok {
		return Price{}, fmt.Errorf("no price for %q in prices.json; add one so cost is reported", model)
	}
	var price Price
	err := json.Unmarshal(raw, &price)
	return price, err
}

func cost(usage Usage, price Price) float64 {
	dollars := (float64(usage.InputTokens)*price.Input + float64(usage.OutputTokens)*price.Output) / 1e6
	return float64(int(dollars*10000+0.5)) / 10000
}

func requireEmptyDir(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("workspace %s is not empty; every model starts from an empty folder", dir)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
