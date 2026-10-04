package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed lint-bugs.yml
var bugsConfig []byte

//go:embed lint-style.yml
var styleConfig []byte

//go:embed lint-cyclo.yml
var cycloConfig []byte

// Measurement is everything a machine can say about one run. It never contains a score
// that depends on a human; tally.go adds those.
type Measurement struct {
	Label       string        `json:"label"` // e.g. claude-sonnet-5-5/run1
	MeasuredAt  time.Time     `json:"measured_at"`
	GoFiles     int           `json:"go_files"`
	CodeLines   int           `json:"code_lines"` // non-test .go lines
	TestLines   int           `json:"test_lines"`
	Build       Build         `json:"build"`
	Hidden      Hidden        `json:"hidden"`
	Style       Style         `json:"style"`
	OwnTests    OwnTests      `json:"own_tests"`
	Findings    []Finding     `json:"findings"` // tool findings; humans give the verdicts
	Elapsed     time.Duration `json:"elapsed_ns"`
	ScorerNotes []string      `json:"scorer_notes,omitempty"`
}

type Build struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
}

type Hidden struct {
	Tests      map[string]string  `json:"tests"`      // leaf test -> pass|fail
	Categories map[string][2]int  `json:"categories"` // category -> [passed, total]
	Points     map[string]float64 `json:"points"`     // category -> points
	Total      float64            `json:"total"`      // out of 40
	Failures   map[string]string  `json:"failures"`   // leaf test -> why (trimmed)
}

type Style struct {
	Issues    int            `json:"issues"`
	ByLinter  map[string]int `json:"by_linter"`
	MaxCyclo  int            `json:"max_cyclo"`
	MaxCycloF string         `json:"max_cyclo_func"`
	Raw       int            `json:"raw"` // issues + max(0, max_cyclo-10); lower is better; ranked in tally
	Details   []string       `json:"details"`
}

type OwnTests struct {
	TestFiles int     `json:"test_files"`
	TestFuncs int     `json:"test_funcs"`
	Passed    bool    `json:"passed"`
	Coverage  float64 `json:"coverage"` // percent of statements
	Points    float64 `json:"points"`   // out of 10
	Output    string  `json:"output,omitempty"`
}

// Finding is one possible bug. Source is the tool (or "agent" for the review agent).
// Verdict starts "pending"; only "confirmed" findings cost points.
type Finding struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Rule     string `json:"rule,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity,omitempty"` // high|medium|low; agent findings only
	Verdict  string `json:"verdict"`            // pending|confirmed|rejected|duplicate
	Note     string `json:"note,omitempty"`
}

// HiddenWeights splits the 40 "Does it work?" points across the test categories,
// so one category with many small cases can't dominate.
var HiddenWeights = map[string]float64{
	"Core":        8,
	"Validation":  8,
	"Admin":       7,
	"Errors":      3,
	"Persistence": 7,
	"Concurrency": 7,
}

func measure(src, acceptance, out, label string) error {
	t0 := time.Now()
	m := &Measurement{Label: label, MeasuredAt: t0.UTC(), Findings: []Finding{}}

	// Work on a copy: the contestant's folder stays exactly as the model left it.
	work, err := os.MkdirTemp("", "score-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	code := filepath.Join(work, "code")
	if err := copyGoProject(src, code); err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	m.GoFiles, m.CodeLines, m.TestLines, m.OwnTests.TestFiles, m.OwnTests.TestFuncs = countGo(code)

	bin := filepath.Join(work, "shortener")
	res := run(code, 5*time.Minute, "go", "build", "-o", bin, ".")
	m.Build = Build{OK: res.err == nil, Output: trimOut(res.out, 4000)}
	if !m.Build.OK {
		m.ScorerNotes = append(m.ScorerNotes, "does not build: every measured category scores 0")
		m.Hidden = Hidden{Tests: map[string]string{}, Categories: map[string][2]int{}, Points: map[string]float64{}}
		for _, t := range expectedTests() {
			m.Hidden.Tests[t] = "fail"
			c := m.Hidden.Categories[category(t)]
			c[1]++
			m.Hidden.Categories[category(t)] = c
		}
		m.Elapsed = time.Since(t0)
		return writeJSON(out, m)
	}

	logf("%s: hidden acceptance tests", label)
	m.Hidden = hiddenTests(acceptance, bin, "", "")

	logf("%s: race detector", label)
	m.Findings = append(m.Findings, raceFindings(code, acceptance, work)...)

	logf("%s: go vet, staticcheck, gosec, errcheck", label)
	m.Findings = append(m.Findings, lintFindings(code, work)...)

	logf("%s: style and complexity", label)
	m.Style = style(code, work)

	logf("%s: its own tests", label)
	m.OwnTests = ownTests(code, work, m.OwnTests)

	for i := range m.Findings {
		m.Findings[i].ID = fmt.Sprintf("T%02d", i+1)
		m.Findings[i].Verdict = "pending"
	}
	m.Elapsed = time.Since(t0)
	return writeJSON(out, m)
}

// hiddenTests runs the acceptance suite against bin and scores it per category.
func hiddenTests(acceptance, bin, runPattern, gorace string) Hidden {
	h := Hidden{Tests: map[string]string{}, Categories: map[string][2]int{}, Points: map[string]float64{}, Failures: map[string]string{}}
	args := []string{"test", "-count=1", "-json", "-timeout", "15m"}
	if runPattern != "" {
		args = append(args, "-run", runPattern)
	}
	args = append(args, "./...")
	env := []string{"SHORTENER_BIN=" + bin}
	if gorace != "" {
		env = append(env, "GORACE="+gorace)
	}
	res := runEnv(acceptance, 20*time.Minute, env, "go", args...)

	outputs := map[string]*strings.Builder{}
	sc := bufio.NewScanner(strings.NewReader(res.out))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var e struct{ Action, Test, Output string }
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Test == "" {
			continue
		}
		switch e.Action {
		case "pass", "fail", "skip":
			h.Tests[e.Test] = e.Action
		case "output":
			if outputs[e.Test] == nil {
				outputs[e.Test] = &strings.Builder{}
			}
			outputs[e.Test].WriteString(e.Output)
		}
	}
	// Keep leaf tests only (a parent with subtests is just their sum).
	for t := range h.Tests {
		if i := strings.LastIndex(t, "/"); i > 0 {
			delete(h.Tests, t[:i])
		}
	}
	// A test that never reported (the suite timed out or crashed) counts as failed.
	for _, name := range expectedTests() {
		if _, ok := h.Tests[name]; !ok && runPattern == "" {
			h.Tests[name] = "fail"
		}
	}
	for t, a := range h.Tests {
		cat := category(t)
		c := h.Categories[cat]
		c[1]++
		if a == "pass" {
			c[0]++
		} else if o := outputs[t]; o != nil {
			h.Failures[t] = failureLine(o.String())
		} else {
			h.Failures[t] = "did not finish (suite timed out or crashed)"
		}
		h.Categories[cat] = c
	}
	for cat, w := range HiddenWeights {
		c := h.Categories[cat]
		if c[1] > 0 {
			h.Points[cat] = round1(w * float64(c[0]) / float64(c[1]))
			h.Total += h.Points[cat]
		}
	}
	h.Total = round1(h.Total)
	return h
}

// expectedTests is the full list of leaf tests, from the reference run (tests.txt).
// It lets a crashed or timed-out suite still count every missing test as a failure.
//
//go:embed tests.txt
var testsTxt string

func expectedTests() []string {
	var out []string
	for _, l := range strings.Split(testsTxt, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func category(test string) string {
	name := strings.TrimPrefix(test, "Test")
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name[:i]
	}
	return "Other"
}

func failureLine(out string) string {
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "=== ") || strings.HasPrefix(l, "--- ") {
			continue
		}
		keep = append(keep, l)
	}
	return trimOut(strings.Join(keep, " | "), 400)
}

// raceFindings builds the program with -race, drives it with the hidden concurrency and core
// tests, and also runs the contestant's own tests with -race. Each distinct race is a finding.
func raceFindings(code, acceptance, work string) []Finding {
	raceBin := filepath.Join(work, "shortener-race")
	if r := runEnv(code, 5*time.Minute, []string{"CGO_ENABLED=1"}, "go", "build", "-race", "-o", raceBin, "."); r.err != nil {
		return []Finding{{Source: "race", Message: "could not build with -race: " + trimOut(r.out, 300)}}
	}
	logDir := filepath.Join(work, "race")
	_ = os.MkdirAll(logDir, 0o755)
	hiddenTests(acceptance, raceBin, "TestConcurrency|TestCore|TestAdmin_Delete", "log_path="+filepath.Join(logDir, "r")+" halt_on_error=0")
	var reports []string
	files, _ := filepath.Glob(filepath.Join(logDir, "r*"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		reports = append(reports, splitRaces(string(b))...)
	}
	own := runEnv(code, 5*time.Minute, []string{"CGO_ENABLED=1"}, "go", "test", "-race", "-count=1", "./...")
	reports = append(reports, splitRaces(own.out)...)

	seen := map[string]bool{}
	var out []Finding
	for _, r := range reports {
		key, file, line := raceKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Finding{Source: "race", Rule: "DATA RACE", File: file, Line: line, Message: trimOut(r, 1500)})
	}
	return out
}

func splitRaces(s string) []string {
	parts := strings.Split(s, "WARNING: DATA RACE")
	var out []string
	for _, p := range parts[1:] {
		if i := strings.Index(p, "=================="); i >= 0 {
			p = p[:i]
		}
		out = append(out, "WARNING: DATA RACE"+p)
	}
	return out
}

var frameRE = regexp.MustCompile(`(?m)^\s+(/\S+\.go):(\d+)`)

// raceKey identifies a race by the first two code locations in the contestant's own files.
func raceKey(report string) (key, file string, line int) {
	var locs []string
	for _, m := range frameRE.FindAllStringSubmatch(report, -1) {
		if strings.Contains(m[1], "/go/src/") || strings.Contains(m[1], "/usr/local/go/") || strings.Contains(m[1], "/pkg/mod/") {
			continue
		}
		loc := filepath.Base(m[1]) + ":" + m[2]
		if file == "" {
			file = filepath.Base(m[1])
			line, _ = strconv.Atoi(m[2])
		}
		locs = append(locs, loc)
		if len(locs) == 2 {
			break
		}
	}
	sort.Strings(locs)
	return strings.Join(locs, "|"), file, line
}

type lintIssue struct {
	FromLinter string
	Text       string
	Pos        struct {
		Filename string
		Line     int
	}
}

func golangciLint(code, work, name string, config []byte) ([]lintIssue, error) {
	cfg := filepath.Join(work, name+".yml")
	if err := os.WriteFile(cfg, config, 0o644); err != nil {
		return nil, err
	}
	res := run(code, 5*time.Minute, "golangci-lint", "run", "--config", cfg,
		"--output.json.path=stdout", "--output.text.path=stderr", "--show-stats=false", "./...")
	line, _, _ := strings.Cut(res.stdout, "\n")
	var parsed struct{ Issues []lintIssue }
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return nil, fmt.Errorf("golangci-lint (%s): %v\n%s", name, err, trimOut(res.out, 1000))
	}
	return parsed.Issues, nil
}

func lintFindings(code, work string) []Finding {
	issues, err := golangciLint(code, work, "bugs", bugsConfig)
	if err != nil {
		return []Finding{{Source: "scorer", Message: err.Error()}}
	}
	var out []Finding
	for _, i := range issues {
		src, rule := i.FromLinter, ""
		if f := strings.Fields(i.Text); len(f) > 0 && (strings.HasPrefix(f[0], "G") || strings.HasPrefix(f[0], "SA")) {
			rule = strings.TrimSuffix(f[0], ":")
		}
		if src == "govet" {
			src = "go vet"
		}
		out = append(out, Finding{Source: src, Rule: rule, File: i.Pos.Filename, Line: i.Pos.Line, Message: i.Text})
	}
	return out
}

var cycloRE = regexp.MustCompile(`cyclomatic complexity (\d+) of func (\S+)`)

func style(code, work string) Style {
	st := Style{ByLinter: map[string]int{}}
	issues, err := golangciLint(code, work, "style", styleConfig)
	if err != nil {
		st.Details = append(st.Details, err.Error())
	}
	for _, i := range issues {
		st.Issues++
		st.ByLinter[i.FromLinter]++
		st.Details = append(st.Details, fmt.Sprintf("%s:%d %s (%s)", i.Pos.Filename, i.Pos.Line, i.Text, i.FromLinter))
	}
	cyclo, _ := golangciLint(code, work, "cyclo", cycloConfig)
	for _, i := range cyclo {
		if m := cycloRE.FindStringSubmatch(i.Text); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > st.MaxCyclo {
				st.MaxCyclo, st.MaxCycloF = n, m[2]
			}
		}
	}
	st.Raw = st.Issues + max(0, st.MaxCyclo-10)
	return st
}

var coverRE = regexp.MustCompile(`total:\s+\(statements\)\s+([\d.]+)%`)

func ownTests(code, work string, ot OwnTests) OwnTests {
	if ot.TestFiles == 0 {
		ot.Output = "no _test.go files"
		return ot
	}
	profile := filepath.Join(work, "cover.out")
	res := run(code, 5*time.Minute, "go", "test", "-count=1", "-timeout", "4m", "-coverprofile", profile, "./...")
	ot.Passed = res.err == nil
	ot.Output = trimOut(res.out, 3000)
	if c := run(code, time.Minute, "go", "tool", "cover", "-func", profile); c.err == nil {
		if m := coverRE.FindStringSubmatch(c.out); m != nil {
			ot.Coverage, _ = strconv.ParseFloat(m[1], 64)
		}
	}
	if ot.Passed {
		ot.Points = round1(min(10, ot.Coverage/80*10))
	}
	return ot
}

// ─── helpers ────────────────────────────────────────────────────────────────

type result struct {
	out    string // stdout + stderr
	stdout string
	err    error
}

func run(dir string, timeout time.Duration, name string, args ...string) result {
	return runEnv(dir, timeout, nil, name, args...)
}

// runEnv runs a command with a minimal environment: contestant code never sees the
// scorer's environment variables (API keys, cloud credentials).
func runEnv(dir string, timeout time.Duration, extra []string, name string, args ...string) result {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	home := filepath.Join(os.TempDir(), "score-home")
	_ = os.MkdirAll(home, 0o755)
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"GOPATH=" + goEnv("GOPATH"),
		"GOCACHE=" + goEnv("GOCACHE"),
		"GOLANGCI_LINT_CACHE=" + filepath.Join(home, "golangci"),
		"GOPROXY=off",
		"GOFLAGS=-mod=mod",
		"GOTOOLCHAIN=local",
		"CGO_ENABLED=0",
	}, extra...)
	var stdout, all bytes.Buffer
	var mu sync.Mutex // stdout and stderr are copied by two goroutines
	cmd.Stdout = &lockedWriter{mu: &mu, ws: []*bytes.Buffer{&stdout, &all}}
	cmd.Stderr = &lockedWriter{mu: &mu, ws: []*bytes.Buffer{&all}}
	err := cmd.Run()
	if ctx.Err() != nil {
		err = fmt.Errorf("timed out after %s", timeout)
		all.WriteString("\n[scorer] " + err.Error())
	}
	return result{out: all.String(), stdout: stdout.String(), err: err}
}

type lockedWriter struct {
	mu *sync.Mutex
	ws []*bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range w.ws {
		b.Write(p)
	}
	return len(p), nil
}

var goEnvCache = map[string]string{}

func goEnv(key string) string {
	if v, ok := goEnvCache[key]; ok {
		return v
	}
	out, _ := exec.Command("go", "env", key).Output()
	v := strings.TrimSpace(string(out))
	goEnvCache[key] = v
	return v
}

// copyGoProject copies the project, skipping .git, symlinks and anything over 5 MB (binaries).
func copyGoProject(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > 5<<20 {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
}

var testFuncRE = regexp.MustCompile(`(?m)^func (Test|Fuzz)\w*\(`)

func countGo(dir string) (files, code, test, testFiles, testFuncs int) {
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".go" {
			return nil
		}
		b, _ := os.ReadFile(p)
		n := bytes.Count(b, []byte("\n"))
		files++
		if strings.HasSuffix(p, "_test.go") {
			test += n
			testFiles++
			testFuncs += len(testFuncRE.FindAll(b, -1))
		} else {
			code += n
		}
		return nil
	})
	return
}

func trimOut(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + " …[trimmed]"
	}
	return s
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func logf(format string, args ...any) { fmt.Fprintf(os.Stderr, "[score] "+format+"\n", args...) }
