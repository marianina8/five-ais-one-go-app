package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	maxWriteBytes  = 256 << 10 // one write_file call
	maxListedFiles = 300
)

// Toolbox holds the tools every contestant gets, confined to one workspace folder:
// list, read and write files, and go build, go test and go vet. Go commands run through a
// Sandbox, which in the contest is a container with no network and no secrets.
type Toolbox struct {
	root      string // absolute path of the workspace
	sandbox   Sandbox
	maxOutput int // bytes of one tool result the model sees
}

func NewToolbox(root string, sandbox Sandbox, maxOutput int) (*Toolbox, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return nil, err
	}
	return &Toolbox{root: abs, sandbox: sandbox, maxOutput: maxOutput}, nil
}

// Tools describes the tools to the model. Every contestant gets exactly these.
func (tb *Toolbox) Tools() []Tool {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
	schema := func(props map[string]any, required ...string) map[string]any {
		if required == nil {
			required = []string{}
		}
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	packages := str(`Package patterns separated by spaces (default "./...")`)

	return []Tool{
		{"list_files", "List the files under a folder of your workspace, with their sizes.",
			schema(map[string]any{"path": str(`Folder relative to the workspace, e.g. "."`)})},
		{"read_file", "Read a file from your workspace. Lines are numbered.",
			schema(map[string]any{
				"path":       str(`File relative to the workspace, e.g. "main.go"`),
				"start_line": num("First line to read (default 1)"),
				"end_line":   num("Last line to read (default: end of file)"),
			}, "path")},
		{"write_file", "Create or replace a file in your workspace with the given content. Folders are created as needed.",
			schema(map[string]any{
				"path":    str(`File relative to the workspace, e.g. "main.go" or "store/store.go"`),
				"content": str("The complete new content of the file"),
			}, "path", "content")},
		{"go_build", "Run go build in your workspace.", schema(map[string]any{"packages": packages})},
		{"go_test", "Run go test in your workspace (with -count=1 and a 2 minute timeout).",
			schema(map[string]any{
				"packages": packages,
				"run":      str("Only run tests matching this regular expression (go test -run)"),
				"verbose":  map[string]any{"type": "boolean", "description": "Pass -v"},
			})},
		{"go_vet", "Run go vet in your workspace.", schema(map[string]any{"packages": packages})},
	}
}

type toolArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Packages  string `json:"packages"`
	Run       string `json:"run"`
	Verbose   bool   `json:"verbose"`
}

// Run runs one tool call. A failure is reported to the model as the tool's result.
func (tb *Toolbox) Run(ctx context.Context, call ToolCall) ToolResult {
	output, err := tb.dispatch(ctx, call)
	if err != nil {
		return ToolResult{CallID: call.ID, Name: call.Name, Output: "error: " + err.Error(), IsError: true}
	}
	return ToolResult{CallID: call.ID, Name: call.Name, Output: truncate(output, tb.maxOutput)}
}

func (tb *Toolbox) dispatch(ctx context.Context, call ToolCall) (string, error) {
	var args toolArgs
	if len(call.Input) > 0 {
		if err := json.Unmarshal(call.Input, &args); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
	}
	switch call.Name {
	case "list_files":
		return tb.listFiles(args.Path)
	case "read_file":
		return tb.readFile(args.Path, args.StartLine, args.EndLine)
	case "write_file":
		return tb.writeFile(args.Path, args.Content)
	case "go_build", "go_test", "go_vet":
		goArgs, err := goCommand(call.Name, args)
		if err != nil {
			return "", err
		}
		return tb.sandbox.Go(ctx, tb.root, goArgs)
	default:
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
}

// goCommand builds the arguments for go build, go test or go vet.
func goCommand(tool string, args toolArgs) ([]string, error) {
	packages, err := packagePatterns(args.Packages)
	if err != nil {
		return nil, err
	}
	goArgs := []string{strings.TrimPrefix(tool, "go_")}
	if tool == "go_test" {
		goArgs = append(goArgs, "-count=1", "-timeout=120s")
		if args.Verbose {
			goArgs = append(goArgs, "-v")
		}
		if args.Run != "" {
			goArgs = append(goArgs, "-run", args.Run)
		}
	}
	return append(goArgs, packages...), nil
}

// resolve turns a model-supplied path into an absolute path inside the workspace.
// Symlinks are followed first, so a link can't lead outside.
func (tb *Toolbox) resolve(path string) (string, error) {
	if path == "" {
		path = "."
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q must be relative to the workspace", path)
	}
	full := filepath.Join(tb.root, path)
	if real, err := filepath.EvalSymlinks(full); err == nil {
		full = real
	} else if real, err := filepath.EvalSymlinks(filepath.Dir(full)); err == nil {
		full = filepath.Join(real, filepath.Base(full)) // a new file in an existing folder
	}
	rel, err := filepath.Rel(tb.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the workspace", path)
	}
	return full, nil
}

func (tb *Toolbox) listFiles(path string) (string, error) {
	dir, err := tb.resolve(path)
	if err != nil {
		return "", err
	}
	var lines []string
	err = filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if len(lines) == maxListedFiles {
			lines = append(lines, "... (more files not shown)")
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(tb.root, p)
		lines = append(lines, fmt.Sprintf("%s (%d bytes)", filepath.ToSlash(rel), info.Size()))
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "(no files)", nil
	}
	return strings.Join(lines, "\n"), nil
}

func (tb *Toolbox) readFile(path string, start, end int) (string, error) {
	full, err := tb.resolve(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full) // #nosec G304 -- resolve keeps it inside the workspace
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	start = max(start, 1)
	if end < 1 || end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return "", fmt.Errorf("start_line %d is past the end of the file (%d lines)", start, len(lines))
	}
	var out strings.Builder
	for i := start; i <= end; i++ {
		fmt.Fprintf(&out, "%4d| %s\n", i, lines[i-1])
	}
	return out.String(), nil
}

func (tb *Toolbox) writeFile(path, content string) (string, error) {
	if path == "" {
		return "", errors.New("path is required")
	}
	if len(content) > maxWriteBytes {
		return "", fmt.Errorf("content is %d bytes; the limit is %d", len(content), maxWriteBytes)
	}
	full, err := tb.resolve(path)
	if err != nil {
		return "", err
	}
	if full == tb.root {
		return "", errors.New("path must name a file")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %s (%d bytes)", path, len(content)), nil
}

// packagePatterns splits "./... ./store" into patterns, refusing flags, downloads and
// paths outside the workspace.
func packagePatterns(s string) ([]string, error) {
	patterns := strings.Fields(s)
	if len(patterns) == 0 {
		return []string{"./..."}, nil
	}
	for _, p := range patterns {
		if strings.HasPrefix(p, "-") || strings.Contains(p, "@") {
			return nil, fmt.Errorf("%q is not a package pattern like ./... or ./store", p)
		}
		if p != "." && !strings.HasPrefix(p, "./") || slices.Contains(strings.Split(p, "/"), "..") {
			return nil, fmt.Errorf("%q: package patterns must start with ./ and stay inside the workspace", p)
		}
	}
	return patterns, nil
}

// truncate keeps the start and end of long output (errors are usually at the end).
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	head := limit / 2
	tail := limit - head
	return s[:head] + fmt.Sprintf("\n\n[... %d bytes cut ...]\n\n", len(s)-limit) + s[len(s)-tail:]
}
