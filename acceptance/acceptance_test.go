package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var codeRE = regexp.MustCompile(`^[A-Za-z0-9]{7}$`)

// ─── Core: create and follow links ──────────────────────────────────────────

func TestCore_CreateGeneratedCode(t *testing.T) {
	s := start(t, opts{})
	r, l := s.create("https://example.com/some/page", "")
	expectStatus(t, r, 201, "create")
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}
	if !codeRE.MatchString(l.Code) {
		t.Errorf("generated code %q: want 7 characters from [A-Za-z0-9]", l.Code)
	}
	if l.URL != "https://example.com/some/page" {
		t.Errorf("url: got %q", l.URL)
	}
	if l.Visits != 0 {
		t.Errorf("visits: want 0, got %d", l.Visits)
	}
	if _, err := time.Parse(time.RFC3339, l.CreatedAt); err != nil {
		t.Errorf("created_at %q is not RFC 3339: %v", l.CreatedAt, err)
	}
}

func TestCore_CreateWithAlias(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "my-page")
	if l.Code != "my-page" {
		t.Fatalf("code: want my-page, got %q", l.Code)
	}
}

func TestCore_GeneratedCodesUnique(t *testing.T) {
	s := start(t, opts{})
	seen := map[string]bool{}
	for i := range 150 {
		l := s.mustCreate(fmt.Sprintf("https://example.com/%d", i), "")
		if !codeRE.MatchString(l.Code) {
			t.Fatalf("generated code %q: want 7 characters from [A-Za-z0-9]", l.Code)
		}
		if seen[l.Code] {
			t.Fatalf("duplicate code %q after %d links", l.Code, i)
		}
		seen[l.Code] = true
	}
}

func TestCore_Redirect(t *testing.T) {
	s := start(t, opts{})
	target := "https://example.com/a/b?x=1&y=two#frag"
	l := s.mustCreate(target, "")
	r := s.do("GET", "/"+l.Code, nil)
	expectStatus(t, r, 302, "follow link")
	if got := r.Header.Get("Location"); got != target {
		t.Fatalf("Location: want %q, got %q", target, got)
	}
}

func TestCore_RedirectAlias(t *testing.T) {
	s := start(t, opts{})
	s.mustCreate("http://example.org/plain-http", "Go_Lang-1")
	r := s.do("GET", "/Go_Lang-1", nil)
	expectStatus(t, r, 302, "follow alias")
	if got := r.Header.Get("Location"); got != "http://example.org/plain-http" {
		t.Fatalf("Location: got %q", got)
	}
}

func TestCore_RedirectUnknown404(t *testing.T) {
	s := start(t, opts{})
	expectStatus(t, s.do("GET", "/nope123", nil), 404, "unknown code")
}

func TestCore_VisitsCounted(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "")
	for range 3 {
		expectStatus(t, s.do("GET", "/"+l.Code, nil), 302, "follow link")
	}
	got, ok := s.find(l.Code)
	if !ok {
		t.Fatal("link missing from list")
	}
	if got.Visits != 3 {
		t.Fatalf("visits: want 3, got %d", got.Visits)
	}
}

// ─── Validation ─────────────────────────────────────────────────────────────

func TestValidation_BadURLs(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"not_a_url":        "not a url",
		"relative":         "/just/a/path",
		"ftp_scheme":       "ftp://example.com/file",
		"javascript":       "javascript:alert(document.cookie)",
		"data_url":         "data:text/html,<script>alert(1)</script>",
		"no_host":          "https://",
		"header_injection": "https://example.com/\r\nSet-Cookie: session=evil",
		"too_long_2049":    "https://example.com/" + strings.Repeat("a", 2049-len("https://example.com/")),
	}
	for name, u := range cases {
		t.Run(name, func(t *testing.T) {
			s := start(t, opts{})
			r, _ := s.create(u, "")
			expectStatus(t, r, 400, fmt.Sprintf("url %.40q", u))
		})
	}
}

func TestValidation_MaxLengthURLAccepted(t *testing.T) {
	s := start(t, opts{})
	u := "https://example.com/" + strings.Repeat("a", 2048-len("https://example.com/"))
	l := s.mustCreate(u, "")
	if l.URL != u {
		t.Fatal("2048-character URL was changed")
	}
}

func TestValidation_BadAliases(t *testing.T) {
	cases := map[string]string{
		"too_short":    "ab",
		"too_long_33":  strings.Repeat("a", 33),
		"space":        "has space",
		"slash":        "a/b/c",
		"reserved_api": "api",
		"non_ascii":    "café-link",
	}
	for name, alias := range cases {
		t.Run(name, func(t *testing.T) {
			s := start(t, opts{})
			r, _ := s.create("https://example.com/", alias)
			expectStatus(t, r, 400, fmt.Sprintf("alias %q", alias))
		})
	}
}

func TestValidation_EdgeAliasesAccepted(t *testing.T) {
	s := start(t, opts{})
	for _, alias := range []string{"abc", strings.Repeat("Z", 32), "a_b-C9"} {
		l := s.mustCreate("https://example.com/", alias)
		if l.Code != alias {
			t.Fatalf("alias %q came back as %q", alias, l.Code)
		}
	}
}

func TestValidation_DuplicateAlias409(t *testing.T) {
	s := start(t, opts{})
	s.mustCreate("https://example.com/1", "taken")
	r, _ := s.create("https://example.com/2", "taken")
	expectStatus(t, r, 409, "duplicate alias")
	r = s.do("GET", "/taken", nil)
	if r.Header.Get("Location") != "https://example.com/1" {
		t.Fatalf("the first link was overwritten: Location %q", r.Header.Get("Location"))
	}
}

func TestValidation_MalformedJSON(t *testing.T) {
	s := start(t, opts{})
	r := s.do("POST", "/api/links", strings.NewReader(`{"url": "https://example.com"`), "Content-Type", "application/json")
	expectStatus(t, r, 400, "malformed JSON")
}

func TestValidation_UnknownField(t *testing.T) {
	s := start(t, opts{})
	r := s.do("POST", "/api/links", strings.NewReader(`{"url":"https://example.com","visits":1000}`), "Content-Type", "application/json")
	expectStatus(t, r, 400, "unknown field")
}

func TestValidation_MissingURL(t *testing.T) {
	s := start(t, opts{})
	r := s.do("POST", "/api/links", strings.NewReader(`{"alias":"no-url"}`), "Content-Type", "application/json")
	expectStatus(t, r, 400, "missing url")
}

// ─── Admin: auth, list, delete ──────────────────────────────────────────────

func TestAdmin_StartWithoutTokenFails(t *testing.T) {
	cmd, s := command(t, opts{noToken: true})
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("program exited with code 0 without ADMIN_TOKEN; want non-zero")
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("program kept running without ADMIN_TOKEN (should exit non-zero)\n%s", tail(s.output.String()))
	}
}

func TestAdmin_ListNeedsToken(t *testing.T) {
	cases := map[string][]string{
		"no_header":    nil,
		"wrong_token":  {"Authorization", "Bearer wrong-token"},
		"token_prefix": {"Authorization", "Bearer " + adminToken[:len(adminToken)-1]},
		"token_longer": {"Authorization", "Bearer " + adminToken + "x"},
		"no_bearer":    {"Authorization", adminToken},
		"basic_scheme": {"Authorization", "Basic " + adminToken},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			s := start(t, opts{})
			s.mustCreate("https://example.com/private", "")
			r := s.do("GET", "/api/links", nil, h...)
			if bytes.Contains(r.Body, []byte("example.com/private")) {
				t.Fatalf("list leaked links without a valid token (status %d)", r.Status)
			}
			expectStatus(t, r, 401, "list with "+name)
		})
	}
}

func TestAdmin_ListEmptyIsArray(t *testing.T) {
	s := start(t, opts{})
	r := s.do("GET", "/api/links", nil, auth...)
	expectStatus(t, r, 200, "list")
	if got := strings.TrimSpace(string(r.Body)); got != "[]" {
		t.Fatalf("empty list: want [], got %s", trim(r.Body))
	}
}

func TestAdmin_ListOldestFirst(t *testing.T) {
	s := start(t, opts{})
	var want []string
	for i := range 5 {
		want = append(want, s.mustCreate(fmt.Sprintf("https://example.com/%d", i), fmt.Sprintf("order-%d", i)).Code)
		time.Sleep(5 * time.Millisecond)
	}
	links := s.list()
	if len(links) != len(want) {
		t.Fatalf("list: want %d links, got %d", len(want), len(links))
	}
	for i, l := range links {
		if l.Code != want[i] {
			t.Fatalf("list order: want %v, got position %d = %q", want, i, l.Code)
		}
		if l.URL == "" || l.CreatedAt == "" {
			t.Fatalf("list entry is missing fields: %+v", l)
		}
	}
}

func TestAdmin_Delete(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "")
	keep := s.mustCreate("https://example.com/keep", "")
	r := s.do("DELETE", "/api/links/"+l.Code, nil, auth...)
	expectStatus(t, r, 204, "delete")
	expectStatus(t, s.do("GET", "/"+l.Code, nil), 404, "follow deleted link")
	if _, ok := s.find(l.Code); ok {
		t.Fatal("deleted link still listed")
	}
	if _, ok := s.find(keep.Code); !ok {
		t.Fatal("delete removed the wrong link")
	}
}

func TestAdmin_DeleteUnknown404(t *testing.T) {
	s := start(t, opts{})
	expectStatus(t, s.do("DELETE", "/api/links/nope123", nil, auth...), 404, "delete unknown")
}

func TestAdmin_DeleteNeedsToken(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "")
	expectStatus(t, s.do("DELETE", "/api/links/"+l.Code, nil), 401, "delete without token")
	expectStatus(t, s.do("DELETE", "/api/links/"+l.Code, nil, "Authorization", "Bearer nope"), 401, "delete wrong token")
	expectStatus(t, s.do("GET", "/"+l.Code, nil), 302, "link after rejected delete")
}

// ─── Errors: status codes and body limits ───────────────────────────────────

// TestErrors_JSONErrorFormat is the one test that checks the error format, so a model that
// returns plain-text errors loses this test, not every validation test.
func TestErrors_JSONErrorFormat(t *testing.T) {
	s := start(t, opts{})
	s.mustCreate("https://example.com/", "dupe")
	checks := []struct {
		what string
		r    resp
		want int
	}{
		{"bad url", s.do("POST", "/api/links", jsonBody(map[string]string{"url": "nope"})), 400},
		{"bad json", s.do("POST", "/api/links", strings.NewReader("{")), 400},
		{"duplicate alias", s.do("POST", "/api/links", jsonBody(map[string]string{"url": "https://example.com/", "alias": "dupe"})), 409},
		{"no token", s.do("GET", "/api/links", nil), 401},
		{"unknown code", s.do("GET", "/nope123", nil), 404},
	}
	for _, c := range checks {
		expectJSONError(t, c.r, c.want, c.what)
	}
}

func TestErrors_BodyTooLarge413(t *testing.T) {
	s := start(t, opts{})
	big := `{"url":"https://example.com/","alias":"` + strings.Repeat("a", 2<<20) + `"}`
	r, err := s.try("POST", "/api/links", strings.NewReader(big), "Content-Type", "application/json")
	if err != nil {
		t.Fatalf("server dropped the connection instead of answering 413: %v", err)
	}
	expectStatus(t, r, 413, "2 MiB body")
}

func TestErrors_MethodNotAllowed(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "")
	expectStatus(t, s.do("PUT", "/api/links", strings.NewReader(`{}`), auth...), 405, "PUT /api/links")
	expectStatus(t, s.do("PATCH", "/api/links/"+l.Code, strings.NewReader(`{}`), auth...), 405, "PATCH /api/links/{code}")
}

// ─── Persistence ────────────────────────────────────────────────────────────

func TestPersistence_SurvivesRestart(t *testing.T) {
	s := start(t, opts{})
	a := s.mustCreate("https://example.com/a", "alpha")
	b := s.mustCreate("https://example.com/b", "")
	for range 2 {
		s.do("GET", "/"+b.Code, nil)
	}
	s = s.restart(true)
	links := s.list()
	if len(links) != 2 || links[0].Code != a.Code || links[1].Code != b.Code {
		t.Fatalf("after restart: want [%s %s], got %+v", a.Code, b.Code, links)
	}
	if links[1].Visits != 2 {
		t.Fatalf("visits after restart: want 2, got %d", links[1].Visits)
	}
	r := s.do("GET", "/alpha", nil)
	if r.Status != 302 || r.Header.Get("Location") != "https://example.com/a" {
		t.Fatalf("follow after restart: %d %q", r.Status, r.Header.Get("Location"))
	}
}

func TestPersistence_DeleteSurvivesRestart(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "gone")
	expectStatus(t, s.do("DELETE", "/api/links/gone", nil, auth...), 204, "delete")
	s = s.restart(true)
	expectStatus(t, s.do("GET", "/"+l.Code, nil), 404, "deleted link after restart")
}

func TestPersistence_UsesDataFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested-name.json")
	s := start(t, opts{data: path})
	s.mustCreate("https://example.com/", "flagged")
	s = s.restart(true)
	if _, ok := s.find("flagged"); !ok {
		t.Fatal("link lost across restart with -data")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("-data file %s was not written: %v", path, err)
	}
}

func TestPersistence_DefaultDataFile(t *testing.T) {
	s := start(t, opts{data: "default"})
	s.mustCreate("https://example.com/", "deflt")
	s = s.restart(true)
	if _, ok := s.find("deflt"); !ok {
		t.Fatal("link lost across restart with the default -data")
	}
	if _, err := os.Stat(filepath.Join(s.dir, "data.json")); err != nil {
		t.Fatalf("default data.json not written in the working directory: %v", err)
	}
}

func TestPersistence_KillAndRecoverWithoutRestartGrace(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "hard-kill")
	s = s.restart(false) // SIGKILL: nothing gets flushed on the way out
	if _, ok := s.find(l.Code); !ok {
		t.Fatal("link acknowledged with 201 was lost after SIGKILL")
	}
}

// TestPersistence_KillDuringWrites kills the program while it is busy writing, several times.
// A program that rewrites its data file in place sooner or later leaves a half-written file
// and then can't start. Writing a temp file and renaming it over the old one avoids that.
func TestPersistence_KillDuringWrites(t *testing.T) {
	s := start(t, opts{})
	// Make the data file big enough that a rewrite takes a while.
	for i := range 300 {
		s.mustCreate("https://example.com/"+strings.Repeat("p", 1500)+fmt.Sprint(i), "")
	}
	for round := range 8 {
		var stop atomic.Bool
		var wg sync.WaitGroup
		for w := range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; !stop.Load(); i++ {
					_, _ = s.try("POST", "/api/links", jsonBody(map[string]string{
						"url": fmt.Sprintf("https://example.com/r%d-w%d-%d", round, w, i),
					}))
				}
			}()
		}
		time.Sleep(time.Duration(150+round*37) * time.Millisecond)
		s.kill()
		stop.Store(true)
		wg.Wait()
		s = start(t, opts{dir: s.dir, data: s.data}) // fails the test if it can't start
		if n := len(s.list()); n < 300 {
			t.Fatalf("round %d: only %d links after restart; at least 300 were saved before the load", round, n)
		}
	}
}

// ─── Concurrency ────────────────────────────────────────────────────────────

func TestConcurrency_ParallelCreates(t *testing.T) {
	s := start(t, opts{})
	const n = 200
	codes := make([]string, n)
	var wg sync.WaitGroup
	var failed atomic.Int32
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.try("POST", "/api/links", jsonBody(map[string]string{"url": fmt.Sprintf("https://example.com/%d", i)}))
			if err != nil || r.Status != 201 {
				failed.Add(1)
				return
			}
			var l Link
			_ = json.Unmarshal(r.Body, &l)
			codes[i] = l.Code
		}()
	}
	wg.Wait()
	if f := failed.Load(); f > 0 {
		t.Fatalf("%d of %d parallel creates failed", f, n)
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate code %q handed out to two parallel creates", c)
		}
		seen[c] = true
	}
	links := s.list()
	if len(links) != n {
		t.Fatalf("list has %d links after %d successful creates: links were lost", len(links), n)
	}
}

func TestConcurrency_ParallelVisitsExact(t *testing.T) {
	s := start(t, opts{})
	l := s.mustCreate("https://example.com/", "")
	const n = 500
	var wg sync.WaitGroup
	sem := make(chan struct{}, 64)
	var bad atomic.Int32
	for range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r, err := s.try("GET", "/"+l.Code, nil)
			if err != nil || r.Status != 302 {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()
	if b := bad.Load(); b > 0 {
		t.Fatalf("%d of %d parallel visits did not get 302", b, n)
	}
	got, _ := s.find(l.Code)
	if got.Visits != n {
		t.Fatalf("visits: want %d, got %d (lost updates)", n, got.Visits)
	}
	s = s.restart(true)
	got, _ = s.find(l.Code)
	if got.Visits != n {
		t.Fatalf("visits after restart: want %d, got %d", n, got.Visits)
	}
}

func TestConcurrency_SameAliasOnceOnly(t *testing.T) {
	s := start(t, opts{})
	const n = 50
	var created, conflicts atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := s.try("POST", "/api/links", jsonBody(map[string]string{"url": fmt.Sprintf("https://example.com/%d", i), "alias": "race"}))
			if err != nil {
				return
			}
			switch r.Status {
			case 201:
				created.Add(1)
			case 409:
				conflicts.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if created.Load() != 1 || conflicts.Load() != n-1 {
		t.Fatalf("%d racing creates of one alias: want 1×201 and %d×409, got %d×201 and %d×409",
			n, n-1, created.Load(), conflicts.Load())
	}
}

func TestConcurrency_MixedLoadNoServerErrors(t *testing.T) {
	s := start(t, opts{})
	var codes []string
	for i := range 20 {
		codes = append(codes, s.mustCreate(fmt.Sprintf("https://example.com/%d", i), "").Code)
	}
	var wg sync.WaitGroup
	var errs atomic.Int32
	var firstErr atomic.Value
	for w := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 25 {
				var r resp
				var err error
				switch (w + i) % 4 {
				case 0:
					r, err = s.try("POST", "/api/links", jsonBody(map[string]string{"url": fmt.Sprintf("https://example.com/m%d-%d", w, i)}))
				case 1:
					r, err = s.try("GET", "/"+codes[(w*7+i)%len(codes)], nil)
				case 2:
					r, err = s.try("GET", "/api/links", nil, auth...)
				case 3:
					r, err = s.try("DELETE", "/api/links/"+codes[(w*3+i)%len(codes)], nil, auth...)
				}
				if err != nil || r.Status >= 500 {
					errs.Add(1)
					firstErr.CompareAndSwap(nil, fmt.Sprintf("status %d err %v", r.Status, err))
				}
			}
		}()
	}
	wg.Wait()
	if e := errs.Load(); e > 0 {
		t.Fatalf("%d requests failed under mixed load; first: %v", e, firstErr.Load())
	}
	// It must still answer correctly afterwards.
	s.list()
}
