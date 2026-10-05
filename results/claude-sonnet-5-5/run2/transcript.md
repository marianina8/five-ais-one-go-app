# claude-sonnet-5-5/run2

`claude-sonnet-5-5` via anthropic · up to 40 model calls, 20 minutes, 16000 output tokens per call · tools: list_files, read_file, write_file, go_build, go_test, go_vet

## Prompt

> Your workspace is empty. Build the service described in this spec.
> 
> # Build a URL shortener in Go
> 
> This is the only document you get. Build the service it describes in the current folder.
> When you are done, say DONE and summarize what you built in a few sentences.
> 
> ## Rules
> - Go 1.24, **standard library only** (no third-party modules; the network is off).
> - Module name `shortener`. The program is in the folder root: `go build -o shortener .` must work.
> - Write your own tests (`go test ./...`).
> - You have tools to list, read and write files and to run `go build`, `go test` and `go vet`. Nothing else.
> 
> ## Running
> ```
> ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
> ```
> - `-addr` default `:8080`. `-data` default `data.json`.
> - If `ADMIN_TOKEN` is empty or unset, print an error and exit with a non-zero code.
> 
> ## API
> Every response body is JSON with `Content-Type: application/json`, except redirects and 204s.
> Every error is `{"error": "<message>"}` with the status code listed below.
> 
> ### Create a link — `POST /api/links` (no auth)
> Request: `{"url": "https://example.com/page", "alias": "my-page"}` (`alias` is optional)
> 
> - `url` must be an absolute `http` or `https` URL with a host, at most 2048 characters. Otherwise **400**.
> - `alias`, if given, must match `^[A-Za-z0-9_-]{3,32}$` and must not be `api`. Otherwise **400**.
> - Alias already taken: **409**.
> - Without an alias, generate a random code of 7 characters from `[A-Za-z0-9]`, unique among existing links.
> - Malformed JSON or unknown fields: **400**. Request body over 1 MiB: **413**.
> - Success: **201** with `{"code": "...", "url": "...", "created_at": "<RFC 3339>", "visits": 0}`
> 
> ### Follow a link — `GET /{code}` (no auth)
> - **302** with `Location` set to the stored URL, and the link's `visits` goes up by one.
> - Unknown code: **404**.
> 
> ### List links — `GET /api/links` (admin)
> - **200** with a JSON array of every link (same fields as above), oldest first.
> 
> ### Delete a link — `DELETE /api/links/{code}` (admin)
> - **204** on success. Unknown code: **404**. Afterwards `GET /{code}` returns **404**.
> 
> ### Admin auth
> Admin requests send `Authorization: Bearer <ADMIN_TOKEN>`. Missing or wrong token: **401**.
> 
> Any other method on a known path: **405** (a HEAD request may be answered like the matching GET).
> 
> ## Storage and concurrency
> - Links (including visit counts) are stored in the `-data` JSON file and must survive a restart.
> - Every change (a new link, a visit, a delete) is saved to the file before its response is sent.
> - A crash or kill at any moment must never leave a file the service can't start from.
> - The service must be correct when many requests arrive at the same time: no lost links,
>   no lost visit counts, no duplicate codes, no data races.
> 
> 
> Budget: 40 model calls and 20 minutes.

## Step 1 · 36s in · 2294 tokens in, 7047 out · stop: tool_use · 19m24s left

<details><summary>→ write_file <code>go.mod</code> (26 bytes)</summary>

```go
module shortener

go 1.24

```
</details>

<details><summary>→ write_file <code>store.go</code> (3881 bytes)</summary>

```go
package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Visits    int64     `json:"visits"`
}

var (
	ErrExists   = errors.New("alias already taken")
	ErrNotFound = errors.New("not found")
)

type Store struct {
	mu    sync.Mutex
	path  string
	links map[string]*Link
	order []*Link
}

type fileFormat struct {
	Links []*Link `json:"links"`
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, links: map[string]*Link{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	var ff fileFormat
	if err := json.Unmarshal(b, &ff); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, l := range ff.Links {
		if l == nil || l.Code == "" {
			continue
		}
		if _, dup := s.links[l.Code]; dup {
			continue
		}
		s.links[l.Code] = l
		s.order = append(s.order, l)
	}
	return s, nil
}

// saveLocked writes the whole state atomically. Caller holds s.mu.
func (s *Store) saveLocked() error {
	ff := fileFormat{Links: s.order}
	if ff.Links == nil {
		ff.Links = []*Link{}
	}
	b, err := json.Marshal(ff)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".shortener-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(b); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	ok = true
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomCode() (string, error) {
	out := make([]byte, 0, 7)
	buf := make([]byte, 16)
	for len(out) < 7 {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, c := range buf {
			if c >= 248 { // 248 = 62*4, avoid modulo bias
				continue
			}
			out = append(out, alphabet[int(c)%62])
			if len(out) == 7 {
				break
			}
		}
	}
	return string(out), nil
}

func (s *Store) Create(url, alias string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code := alias
	if code != "" {
		if _, ok := s.links[code]; ok {
			return Link{}, ErrExists
		}
	} else {
		for {
			c, err := randomCode()
			if err != nil {
				return Link{}, err
			}
			if _, ok := s.links[c]; !ok && c != "api" {
				code = c
				break
			}
		}
	}
	l := &Link{Code: code, URL: url, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	s.links[code] = l
	s.order = append(s.order, l)
	if err := s.saveLocked(); err != nil {
		delete(s.links, code)
		s.order = s.order[:len(s.order)-1]
		return Link{}, err
	}
	return *l, nil
}

func (s *Store) Visit(code string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	l.Visits++
	if err := s.saveLocked(); err != nil {
		l.Visits--
		return Link{}, err
	}
	return *l, nil
}

func (s *Store) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.links[code]
	if !ok {
		return ErrNotFound
	}
	idx := -1
	for i, x := range s.order {
		if x == l {
			idx = i
			break
		}
	}
	oldOrder := append([]*Link(nil), s.order...)
	delete(s.links, code)
	s.order = append(s.order[:idx], s.order[idx+1:]...)
	if err := s.saveLocked(); err != nil {
		s.links[code] = l
		s.order = oldOrder
		return err
	}
	return nil
}

func (s *Store) List() []Link {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Link, 0, len(s.order))
	for _, l := range s.order {
		out = append(out, *l)
	}
	return out
}

```
</details>

<details><summary>→ write_file <code>main.go</code> (4228 bytes)</summary>

```go
package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)

type Server struct {
	store *Store
	token string
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) &&
		subtle.ConstantTimeCompare([]byte(h[len(p):]), []byte(s.token)) == 1 {
		return true
	}
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeErr(w, http.StatusUnauthorized, "unauthorized")
	return false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/api/links":
		switch r.Method {
		case http.MethodPost:
			s.create(w, r)
		case http.MethodGet, http.MethodHead:
			if s.admin(w, r) {
				writeJSON(w, 200, s.store.List())
			}
		default:
			s.methodNotAllowed(w, "GET, HEAD, POST")
		}
	case strings.HasPrefix(path, "/api/links/") && !strings.Contains(path[len("/api/links/"):], "/"):
		code := path[len("/api/links/"):]
		if r.Method != http.MethodDelete {
			s.methodNotAllowed(w, "DELETE")
			return
		}
		if !s.admin(w, r) {
			return
		}
		if err := s.store.Delete(code); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, 404, "not found")
			} else {
				writeErr(w, 500, "internal error")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case len(path) > 1 && !strings.Contains(path[1:], "/") && path != "/api":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.methodNotAllowed(w, "GET, HEAD")
			return
		}
		l, err := s.store.Visit(path[1:])
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeErr(w, 404, "not found")
			} else {
				writeErr(w, 500, "internal error")
			}
			return
		}
		w.Header().Set("Location", l.URL)
		w.WriteHeader(http.StatusFound)
	default:
		writeErr(w, 404, "not found")
	}
}

type createReq struct {
	URL   string `json:"url"`
	Alias string `json:"alias"`
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req createReq
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, 413, "request body too large")
			return
		}
		writeErr(w, 400, "invalid JSON")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, 413, "request body too large")
			return
		}
		writeErr(w, 400, "invalid JSON")
		return
	}
	if len(req.URL) > 2048 || req.URL == "" {
		writeErr(w, 400, "invalid url")
		return
	}
	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		writeErr(w, 400, "url must be an absolute http or https URL")
		return
	}
	if req.Alias != "" && (!aliasRe.MatchString(req.Alias) || req.Alias == "api") {
		writeErr(w, 400, "invalid alias")
		return
	}
	l, err := s.store.Create(req.URL, req.Alias)
	if err != nil {
		if errors.Is(err, ErrExists) {
			writeErr(w, 409, "alias already taken")
		} else {
			writeErr(w, 500, "internal error")
		}
		return
	}
	writeJSON(w, 201, l)
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	data := flag.String("data", "data.json", "data file")
	flag.Parse()
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: ADMIN_TOKEN must be set")
		os.Exit(1)
	}
	st, err := OpenStore(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	log.Fatal(http.ListenAndServe(*addr, &Server{store: st, token: token}))
}

```
</details>

<details><summary>→ write_file <code>main_test.go</code> (3139 bytes)</summary>

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newSrv(t *testing.T, path string) *Server {
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}
}

func do(s *Server, method, path, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth {
		r.Header.Set("Authorization", "Bearer secret")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestFlow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	s := newSrv(t, p)
	w := do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if w := do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false); w.Code != 409 {
		t.Fatal(w.Code)
	}
	for _, b := range []string{`{"url":"ftp://a.com"}`, `{"url":"/x"}`, `{"url":"https://a.com","alias":"api"}`,
		`{"url":"https://a.com","alias":"a b"}`, `{"url":"https://a.com","x":1}`, `nope`, `{"url":"https://a.com"} x`} {
		if w := do(s, "POST", "/api/links", b, false); w.Code != 400 {
			t.Fatal(b, w.Code)
		}
	}
	if w := do(s, "POST", "/api/links", `{"url":"https://a.com/`+strings.Repeat("a", 2<<20)+`"}`, false); w.Code != 413 {
		t.Fatal(w.Code)
	}
	w = do(s, "POST", "/api/links", `{"url":"https://a.com"}`, false)
	var l Link
	json.Unmarshal(w.Body.Bytes(), &l)
	if len(l.Code) != 7 {
		t.Fatal(l)
	}
	if w := do(s, "GET", "/my-page", "", false); w.Code != 302 || w.Header().Get("Location") != "https://example.com/x" {
		t.Fatal(w.Code)
	}
	if w := do(s, "GET", "/nope", "", false); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(s, "GET", "/api/links", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := do(s, "PUT", "/api/links", "", true); w.Code != 405 {
		t.Fatal(w.Code)
	}
	if w := do(s, "POST", "/my-page", "", false); w.Code != 405 {
		t.Fatal(w.Code)
	}
	// restart
	s = newSrv(t, p)
	w = do(s, "GET", "/api/links", "", true)
	var ls []Link
	json.Unmarshal(w.Body.Bytes(), &ls)
	if len(ls) != 2 || ls[0].Code != "my-page" || ls[0].Visits != 1 {
		t.Fatal(w.Body)
	}
	if w := do(s, "DELETE", "/api/links/my-page", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := do(s, "DELETE", "/api/links/my-page", "", true); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := do(s, "DELETE", "/api/links/my-page", "", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(s, "GET", "/my-page", "", false); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestConcurrent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	s := newSrv(t, p)
	do(s, "POST", "/api/links", `{"url":"https://a.com","alias":"hot"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			do(s, "POST", "/api/links", fmt.Sprintf(`{"url":"https://a.com/%d"}`, i), false)
			do(s, "GET", "/hot", "", false)
		}(i)
	}
	wg.Wait()
	s = newSrv(t, p)
	ls := s.store.List()
	if len(ls) != 41 || ls[0].Visits != 40 {
		t.Fatal(len(ls), ls[0].Visits)
	}
}

```
</details>

<details><summary>write_file result (23 bytes)</summary>

```text
wrote go.mod (26 bytes)
```
</details>

<details><summary>write_file result (27 bytes)</summary>

```text
wrote store.go (3881 bytes)
```
</details>

<details><summary>write_file result (26 bytes)</summary>

```text
wrote main.go (4228 bytes)
```
</details>

<details><summary>write_file result (31 bytes)</summary>

```text
wrote main_test.go (3139 bytes)
```
</details>

_Budget: 39 model calls and 19m24s left._

## Step 2 · 38s in · 9527 tokens in, 75 out · stop: tool_use · 19m22s left

→ `go_vet` `{}`

→ `go_test` `{"packages":"./... -race"}`

<details><summary>go_vet result (121 bytes)</summary>

```text
$ go vet ./...
exit code 1 after 11s
# shortener
# [shortener]
vet: ./main_test.go:6:2: "net/http" imported and not used

```
</details>

<details><summary>go_test error (61 bytes)</summary>

```text
error: "-race" is not a package pattern like ./... or ./store
```
</details>

_Budget: 38 model calls and 19m11s left._

## Step 3 · 57s in · 9760 tokens in, 1835 out · stop: tool_use · 19m3s left

<details><summary>→ write_file <code>main_test.go</code> (3127 bytes)</summary>

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newSrv(t *testing.T, path string) *Server {
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}
}

func do(s *Server, method, path, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth {
		r.Header.Set("Authorization", "Bearer secret")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestFlow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	s := newSrv(t, p)
	w := do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if w := do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false); w.Code != 409 {
		t.Fatal(w.Code)
	}
	for _, b := range []string{`{"url":"ftp://a.com"}`, `{"url":"/x"}`, `{"url":"https://a.com","alias":"api"}`,
		`{"url":"https://a.com","alias":"a b"}`, `{"url":"https://a.com","x":1}`, `nope`, `{"url":"https://a.com"} x`} {
		if w := do(s, "POST", "/api/links", b, false); w.Code != 400 {
			t.Fatal(b, w.Code)
		}
	}
	if w := do(s, "POST", "/api/links", `{"url":"https://a.com/`+strings.Repeat("a", 2<<20)+`"}`, false); w.Code != 413 {
		t.Fatal(w.Code)
	}
	w = do(s, "POST", "/api/links", `{"url":"https://a.com"}`, false)
	var l Link
	json.Unmarshal(w.Body.Bytes(), &l)
	if len(l.Code) != 7 {
		t.Fatal(l)
	}
	if w := do(s, "GET", "/my-page", "", false); w.Code != 302 || w.Header().Get("Location") != "https://example.com/x" {
		t.Fatal(w.Code)
	}
	if w := do(s, "GET", "/nope", "", false); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(s, "GET", "/api/links", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := do(s, "PUT", "/api/links", "", true); w.Code != 405 {
		t.Fatal(w.Code)
	}
	if w := do(s, "POST", "/my-page", "", false); w.Code != 405 {
		t.Fatal(w.Code)
	}
	// restart
	s = newSrv(t, p)
	w = do(s, "GET", "/api/links", "", true)
	var ls []Link
	json.Unmarshal(w.Body.Bytes(), &ls)
	if len(ls) != 2 || ls[0].Code != "my-page" || ls[0].Visits != 1 {
		t.Fatal(w.Body)
	}
	if w := do(s, "DELETE", "/api/links/my-page", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := do(s, "DELETE", "/api/links/my-page", "", true); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := do(s, "DELETE", "/api/links/my-page", "", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := do(s, "GET", "/my-page", "", false); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestConcurrent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	s := newSrv(t, p)
	do(s, "POST", "/api/links", `{"url":"https://a.com","alias":"hot"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			do(s, "POST", "/api/links", fmt.Sprintf(`{"url":"https://a.com/%d"}`, i), false)
			do(s, "GET", "/hot", "", false)
		}(i)
	}
	wg.Wait()
	s = newSrv(t, p)
	ls := s.store.List()
	if len(ls) != 41 || ls[0].Visits != 40 {
		t.Fatal(len(ls), ls[0].Visits)
	}
}

```
</details>

<details><summary>write_file result (31 bytes)</summary>

```text
wrote main_test.go (3127 bytes)
```
</details>

_Budget: 37 model calls and 19m3s left._

## Step 4 · 58s in · 11638 tokens in, 50 out · stop: tool_use · 19m2s left

→ `go_vet` `{}`

→ `go_test` `{}`

<details><summary>go_vet result (36 bytes)</summary>

```text
$ go vet ./...
exit code 0 after 0s

```
</details>

<details><summary>go_test result (82 bytes)</summary>

```text
$ go test -count=1 -timeout=120s ./...
exit code 0 after 3s
ok  	shortener	0.080s

```
</details>

_Budget: 36 model calls and 18m59s left._

## Step 5 · 1m6s in · 11831 tokens in, 496 out · stop: end_turn · 18m54s left

> DONE. I built the URL shortener as specified, using only the standard library. `go vet` is clean and `go test` passes. I didn't get a `-race` run: the test tool rejected the flag. I also never ran `go build -o shortener .` or started the binary. The tests call the handler directly, so the flags and the `ADMIN_TOKEN` exit path haven't been exercised.
> 
> - **API:** All the specified routes and status codes are there (create, follow, list, delete), with 405 on wrong methods and 401 on a missing or wrong token. The method check comes before the auth check. `HEAD` is treated like `GET`, so a `HEAD` on a short link also counts as a visit.
> - **Storage:** State lives in the `-data` JSON file. Every create, visit and delete is saved before the response is sent. A save writes a temp file, syncs it, then renames it over the data file, so a crash can't leave a broken file. If a save fails, the change is rolled back and the request gets a 500.
> - **Concurrency:** One mutex covers the in-memory state and the file write, so concurrent requests can't lose links or visit counts or produce duplicate codes. Visits are therefore written one at a time.
> - **Tests:** `main_test.go` covers the full flow, validation errors, the 413 limit, persistence across a restart, and a concurrent test with 40 parallel creates and visits that checks nothing is lost after reloading from disk.
> 
> The code is in `main.go` (HTTP handling) and `store.go` (storage).

## Stopped: done after 5 model calls and 1m6s
