package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ProjectSize counts a project's Go files and lines.
type ProjectSize struct {
	GoFiles   int `json:"go_files"`
	CodeLines int `json:"code_lines"` // lines in non-test .go files
	TestLines int `json:"test_lines"`
	TestFiles int `json:"test_files"`
	TestFuncs int `json:"test_funcs"` // Test and Fuzz functions
}

const maxCopiedFileSize = 5 << 20 // bigger files are build outputs, not source

// copyProject copies src to dst, skipping .git, symlinks and files over 5 MB.
func copyProject(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o750)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if info, err := entry.Info(); err != nil || info.Size() > maxCopiedFileSize {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 -- reads a file under a folder the operator passed in
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600)
	})
}

var testFuncPattern = regexp.MustCompile(`(?m)^func (Test|Fuzz)\w*\(`)

func measureSize(dir string) ProjectSize {
	var size ProjectSize
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		source, err := os.ReadFile(path) // #nosec G304 -- reads a file under a folder the operator passed in
		if err != nil {
			return nil
		}
		lines := bytes.Count(source, []byte("\n"))
		size.GoFiles++
		if strings.HasSuffix(path, "_test.go") {
			size.TestFiles++
			size.TestLines += lines
			size.TestFuncs += len(testFuncPattern.FindAll(source, -1))
		} else {
			size.CodeLines += lines
		}
		return nil
	})
	return size
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path) // #nosec G304 -- reads a file under a folder the operator passed in
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// truncate trims s and cuts it to at most limit bytes.
func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) > limit {
		return s[:limit] + " …[trimmed]"
	}
	return s
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func logf(format string, args ...any) { fmt.Fprintf(os.Stderr, "[score] "+format+"\n", args...) }
