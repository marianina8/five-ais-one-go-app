package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Sandbox runs a go command in the workspace. The command runs the contestant's code
// (go test), so it gets no API keys and no network.
type Sandbox interface {
	Go(ctx context.Context, workspace string, args []string) (string, error)
}

// goEnv is the environment for go commands: enough to build and test offline, nothing else.
var goEnv = []string{"GOPROXY=off", "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOTELEMETRY=off"}

// DockerSandbox runs each go command in a fresh container: no network, the workspace as the
// only mounted folder, and limits on memory and processes. This is what the contest uses.
// The container runs as the harness's own user, so files stay readable and writable by both.
type DockerSandbox struct {
	Image    string // e.g. golang:1.24
	CacheDir string // build cache shared between commands, outside the workspace
	Timeout  time.Duration
}

func (d *DockerSandbox) Go(ctx context.Context, workspace string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()
	name := "contestant-" + randomHex(6)
	dockerArgs := []string{
		"run", "--rm", "--name", name,
		"--network", "none",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--memory", "4g", "--pids-limit", "1024",
		"-v", workspace + ":/work", "-w", "/work",
		"-v", d.CacheDir + ":/gocache",
		"-e", "HOME=/tmp", "-e", "GOCACHE=/gocache", "-e", "GOPATH=/tmp/gopath",
	}
	for _, kv := range goEnv {
		dockerArgs = append(dockerArgs, "-e", kv)
	}
	dockerArgs = append(append(dockerArgs, d.Image, "go"), args...)

	// The docker client gets only what it needs to find docker: nothing from the harness leaks in.
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...) // #nosec G204 -- a fixed binary; arguments are a list, never a shell string
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+host)
	}
	cmd.WaitDelay = 10 * time.Second
	output, err := runCommand(cmd, ctx, "go "+strings.Join(args, " "), d.Timeout)
	if ctx.Err() != nil {
		// Killing the docker client doesn't stop the container; remove it explicitly.
		_ = exec.Command("docker", "rm", "-f", name).Run() // #nosec G204 -- our own generated name
	}
	return output, err
}

// LocalSandbox runs go directly with a stripped environment. It is for trying the harness
// on your own machine; it doesn't isolate the contestant's code from the network.
type LocalSandbox struct {
	Timeout time.Duration
}

func (l *LocalSandbox) Go(ctx context.Context, workspace string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = workspace
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.TempDir()}, goEnv...)
	for _, name := range []string{"GOCACHE", "GOROOT", "TMPDIR"} {
		if value := os.Getenv(name); value != "" {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	killProcessGroupOnCancel(cmd)
	output, err := runCommand(cmd, ctx, "go "+strings.Join(args, " "), l.Timeout)
	killLeftovers(cmd)
	return output, err
}

// runCommand runs cmd and formats what the model sees: the command, exit code, time and
// output. A non-zero exit (failing tests, compile errors) is a normal result, not an error.
func runCommand(cmd *exec.Cmd, ctx context.Context, display string, timeout time.Duration) (string, error) {
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	started := time.Now()
	err := cmd.Run()
	took := time.Since(started).Round(time.Second)
	switch {
	case ctx.Err() != nil:
		return fmt.Sprintf("$ %s\ntimed out after %s\n%s", display, timeout, out.String()), nil
	case err != nil && cmd.ProcessState == nil:
		return "", fmt.Errorf("could not run %s: %w", display, err)
	}
	return fmt.Sprintf("$ %s\nexit code %d after %s\n%s", display, cmd.ProcessState.ExitCode(), took, out.String()), nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never fails on supported platforms
	return hex.EncodeToString(b)
}
