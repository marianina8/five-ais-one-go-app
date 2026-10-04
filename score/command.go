package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type commandResult struct {
	output string // stdout and stderr, interleaved
	stdout string
	err    error
}

func run(dir string, timeout time.Duration, name string, args ...string) commandResult {
	return runWithEnv(dir, timeout, nil, name, args...)
}

// runWithEnv runs a command with a minimal environment plus extraEnv. Contestant code runs
// this way, so it never sees the scorer's environment (API keys, cloud credentials).
// The command runs in its own process group, which is killed when it finishes or times out,
// so a server or background process the contestant's code started can't outlive it.
func runWithEnv(dir string, timeout time.Duration, extraEnv []string, name string, args ...string) commandResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	home := filepath.Join(os.TempDir(), "score-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return commandResult{err: err}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
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
	}, extraEnv...) // later entries win, so extraEnv can override
	killProcessGroupOnCancel(cmd)

	var stdout, combined bytes.Buffer
	var mu sync.Mutex // os/exec copies stdout and stderr in two goroutines
	cmd.Stdout = &lockedWriter{mu: &mu, targets: []*bytes.Buffer{&stdout, &combined}}
	cmd.Stderr = &lockedWriter{mu: &mu, targets: []*bytes.Buffer{&combined}}

	err := cmd.Run()
	killLeftovers(cmd)
	if ctx.Err() != nil {
		err = fmt.Errorf("timed out after %s", timeout)
		mu.Lock()
		combined.WriteString("\n[scorer] " + err.Error())
		mu.Unlock()
	}
	return commandResult{output: combined.String(), stdout: stdout.String(), err: err}
}

// lockedWriter writes to several buffers under one shared mutex.
type lockedWriter struct {
	mu      *sync.Mutex
	targets []*bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, target := range w.targets {
		target.Write(p) // bytes.Buffer.Write never returns an error
	}
	return len(p), nil
}

var (
	goEnvMu    sync.Mutex
	goEnvCache = map[string]string{}
)

// goEnv returns `go env key`, asking the go command once per key.
func goEnv(key string) string {
	goEnvMu.Lock()
	defer goEnvMu.Unlock()
	if value, ok := goEnvCache[key]; ok {
		return value
	}
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		logf("go env %s: %v", key, err)
	}
	value := strings.TrimSpace(string(out))
	goEnvCache[key] = value
	return value
}
