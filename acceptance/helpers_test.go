// Hidden acceptance tests for SPEC.md. Never shown to a contestant.
//
// They treat the contestant's program as a black box: build it, start it, talk to it over HTTP.
// Every test starts its own fresh server, so one failure can't cascade into others.
//
//	SHORTENER_BIN=/path/to/shortener go test -json ./...
//
// Test names start with a category (Core, Validation, Admin, Errors, Persistence, Concurrency);
// score/score.go weights the categories.
package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const adminToken = "hidden-acceptance-Tok3n-7f3a"

var client = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse // never follow redirects; we check them
	},
	Transport: &http.Transport{MaxIdleConnsPerHost: 200, MaxConnsPerHost: 200},
}

// server is one running copy of the contestant's program.
type server struct {
	t      *testing.T
	dir    string // working directory (and home of the default data file)
	data   string // -data path; "" means: don't pass -data
	base   string
	cmd    *exec.Cmd
	output *bytes.Buffer
	done   chan struct{} // closed when the process has exited
}

type opts struct {
	dir     string // reuse a working directory (for restarts); "" = new temp dir
	data    string // -data value; "default" = omit the flag
	noToken bool   // start without ADMIN_TOKEN
}

func bin(t *testing.T) string {
	t.Helper()
	b := os.Getenv("SHORTENER_BIN")
	if b == "" {
		t.Fatal("SHORTENER_BIN is not set")
	}
	abs, err := filepath.Abs(b)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// command builds the exec.Cmd without starting it. The program gets a minimal environment:
// none of the scorer's variables (and so no secrets) reach contestant code.
func command(t *testing.T, o opts) (*exec.Cmd, *server) {
	t.Helper()
	s := &server{t: t, dir: o.dir, output: &bytes.Buffer{}}
	if s.dir == "" {
		s.dir = t.TempDir()
	}
	port := freePort(t)
	args := []string{"-addr", fmt.Sprintf("127.0.0.1:%d", port)}
	switch o.data {
	case "default":
	case "":
		s.data = filepath.Join(s.dir, "links.json")
		args = append(args, "-data", s.data)
	default:
		s.data = o.data
		args = append(args, "-data", s.data)
	}
	cmd := exec.Command(bin(t), args...)
	cmd.Dir = s.dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + s.dir, "TMPDIR=" + s.dir}
	if !o.noToken {
		cmd.Env = append(cmd.Env, "ADMIN_TOKEN="+adminToken)
	}
	if v := os.Getenv("GORACE"); v != "" {
		cmd.Env = append(cmd.Env, "GORACE="+v) // the scorer's race run collects reports this way
	}
	cmd.Stdout, cmd.Stderr = s.output, s.output
	s.cmd = cmd
	s.base = fmt.Sprintf("http://127.0.0.1:%d", port)
	return cmd, s
}

// start runs the program and waits until it accepts connections.
func start(t *testing.T, o opts) *server {
	t.Helper()
	cmd, s := command(t, o)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan struct{})
	s.done = done
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			t.Fatalf("program exited during startup:\n%s", tail(s.output.String()))
		default:
		}
		conn, err := net.DialTimeout("tcp", strings.TrimPrefix(s.base, "http://"), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("program did not start listening within 10s:\n%s", tail(s.output.String()))
	return nil
}

// stop asks the program to shut down (SIGTERM), then kills it after 5 s.
func (s *server) stop() {
	_ = s.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		_ = s.cmd.Process.Kill()
		<-s.done
	}
}

// kill stops the program with SIGKILL: no chance to clean up.
func (s *server) kill() {
	_ = s.cmd.Process.Kill()
	<-s.done
}

// restart stops the program and starts a new one on the same working directory and data file.
func (s *server) restart(graceful bool) *server {
	if graceful {
		s.stop()
	} else {
		s.kill()
	}
	o := opts{dir: s.dir, data: s.data}
	if s.data == "" {
		o.data = "default"
	}
	return start(s.t, o)
}

type resp struct {
	Status int
	Header http.Header
	Body   []byte
}

func (s *server) do(method, path string, body io.Reader, headers ...string) resp {
	s.t.Helper()
	r, err := s.try(method, path, body, headers...)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	return r
}

func (s *server) try(method, path string, body io.Reader, headers ...string) (resp, error) {
	req, err := http.NewRequest(method, s.base+path, body)
	if err != nil {
		return resp{}, err
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := client.Do(req)
	if err != nil {
		return resp{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Header: res.Header, Body: b}, err
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

var auth = []string{"Authorization", "Bearer " + adminToken}

// Link is the shape every endpoint returns.
type Link struct {
	Code      string `json:"code"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	Visits    int64  `json:"visits"`
}

func (s *server) create(url, alias string) (resp, Link) {
	s.t.Helper()
	req := map[string]string{"url": url}
	if alias != "" {
		req["alias"] = alias
	}
	r := s.do("POST", "/api/links", jsonBody(req), "Content-Type", "application/json")
	var l Link
	if r.Status == http.StatusCreated {
		if err := json.Unmarshal(r.Body, &l); err != nil {
			s.t.Fatalf("201 body is not a link: %v\n%s", err, r.Body)
		}
	}
	return r, l
}

func (s *server) mustCreate(url, alias string) Link {
	s.t.Helper()
	r, l := s.create(url, alias)
	if r.Status != http.StatusCreated {
		s.t.Fatalf("create %q (alias %q): want 201, got %d: %s", url, alias, r.Status, r.Body)
	}
	return l
}

func (s *server) list() []Link {
	s.t.Helper()
	r := s.do("GET", "/api/links", nil, auth...)
	if r.Status != http.StatusOK {
		s.t.Fatalf("list: want 200, got %d: %s", r.Status, r.Body)
	}
	var links []Link
	if err := json.Unmarshal(r.Body, &links); err != nil {
		s.t.Fatalf("list body is not a JSON array of links: %v\n%s", err, r.Body)
	}
	return links
}

func (s *server) find(code string) (Link, bool) {
	for _, l := range s.list() {
		if l.Code == code {
			return l, true
		}
	}
	return Link{}, false
}

func expectStatus(t *testing.T, r resp, want int, what string) {
	t.Helper()
	if r.Status != want {
		t.Fatalf("%s: want %d, got %d: %s", what, want, r.Status, trim(r.Body))
	}
}

// expectJSONError checks the spec's error format: {"error": "<message>"} as application/json.
func expectJSONError(t *testing.T, r resp, want int, what string) {
	t.Helper()
	expectStatus(t, r, want, what)
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("%s: error Content-Type should be application/json, got %q", what, ct)
	}
	var e map[string]any
	if err := json.Unmarshal(r.Body, &e); err != nil {
		t.Fatalf("%s: error body is not JSON: %s", what, trim(r.Body))
	}
	if msg, _ := e["error"].(string); msg == "" {
		t.Fatalf("%s: error body needs a non-empty \"error\" string: %s", what, trim(r.Body))
	}
}

func trim(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "..."
	}
	return string(b)
}

func tail(s string) string {
	if len(s) > 2000 {
		return "..." + s[len(s)-2000:]
	}
	return s
}
