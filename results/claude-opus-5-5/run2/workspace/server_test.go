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

func setup(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.json")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(NewServer(st, "secret"))
	t.Cleanup(ts.Close)
	return ts, path
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func do(t *testing.T, method, url, body string, admin bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if admin {
		req.Header.Set("Authorization", "Bearer secret")
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestFlow(t *testing.T) {
	ts, path := setup(t)
	r := do(t, "POST", ts.URL+"/api/links", `{"url":"https://example.com/a","alias":"my-page"}`, false)
	if r.StatusCode != 201 || r.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("create: %d", r.StatusCode)
	}
	var l Link
	json.NewDecoder(r.Body).Decode(&l)
	if l.Code != "my-page" || l.Visits != 0 {
		t.Fatalf("bad link %+v", l)
	}
	if r := do(t, "POST", ts.URL+"/api/links", `{"url":"https://example.com/b","alias":"my-page"}`, false); r.StatusCode != 409 {
		t.Fatalf("dup: %d", r.StatusCode)
	}
	r = do(t, "POST", ts.URL+"/api/links", `{"url":"http://x.org"}`, false)
	json.NewDecoder(r.Body).Decode(&l)
	if r.StatusCode != 201 || len(l.Code) != 7 {
		t.Fatalf("gen: %d %+v", r.StatusCode, l)
	}
	r = do(t, "GET", ts.URL+"/my-page", "", false)
	if r.StatusCode != 302 || r.Header.Get("Location") != "https://example.com/a" {
		t.Fatalf("redirect: %d", r.StatusCode)
	}
	if r := do(t, "GET", ts.URL+"/api/links", "", false); r.StatusCode != 401 {
		t.Fatalf("auth: %d", r.StatusCode)
	}
	r = do(t, "GET", ts.URL+"/api/links", "", true)
	var list []Link
	json.NewDecoder(r.Body).Decode(&list)
	if len(list) != 2 || list[0].Code != "my-page" || list[0].Visits != 1 {
		t.Fatalf("list: %+v", list)
	}
	// restart
	st2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := st2.List(); len(got) != 2 || got[0].Visits != 1 {
		t.Fatalf("reload: %+v", got)
	}
	if r := do(t, "DELETE", ts.URL+"/api/links/my-page", "", true); r.StatusCode != 204 {
		t.Fatalf("delete: %d", r.StatusCode)
	}
	if r := do(t, "DELETE", ts.URL+"/api/links/my-page", "", true); r.StatusCode != 404 {
		t.Fatalf("delete2: %d", r.StatusCode)
	}
	if r := do(t, "GET", ts.URL+"/my-page", "", false); r.StatusCode != 404 {
		t.Fatalf("get deleted: %d", r.StatusCode)
	}
	if r := do(t, "PUT", ts.URL+"/api/links", "", true); r.StatusCode != 405 {
		t.Fatalf("405: %d", r.StatusCode)
	}
}

func TestValidation(t *testing.T) {
	ts, _ := setup(t)
	cases := map[string]int{
		`{"url":"ftp://x.com"}`:                      400,
		`{"url":"/relative"}`:                        400,
		`{"url":"https://"}`:                         400,
		`{"url":"https://x.com","alias":"ab"}`:       400,
		`{"url":"https://x.com","alias":"api"}`:      400,
		`{"url":"https://x.com","alias":"a b c"}`:    400,
		`{"url":"https://x.com","extra":1}`:          400,
		`{"url":`:                                    400,
		`{}`:                                         400,
		`{"url":"https://x.com/` + strings.Repeat("a", 2048) + `"}`: 400,
		`{"url":"https://x.com","pad":"` + strings.Repeat("a", 2<<20) + `"}`: 413,
	}
	for body, want := range cases {
		r := do(t, "POST", ts.URL+"/api/links", body, false)
		if r.StatusCode != want {
			t.Errorf("%.60s: got %d want %d", body, r.StatusCode, want)
		}
	}
}

func TestConcurrent(t *testing.T) {
	ts, path := setup(t)
	do(t, "POST", ts.URL+"/api/links", `{"url":"https://example.com","alias":"hot"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			req, _ := http.NewRequest("POST", ts.URL+"/api/links", strings.NewReader(fmt.Sprintf(`{"url":"https://e.com/%d"}`, i)))
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}(i)
		go func() {
			defer wg.Done()
			resp, err := noRedirect.Get(ts.URL + "/hot")
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	list := st.List()
	if len(list) != 51 || list[0].Visits != 50 {
		t.Fatalf("got %d links, visits %d", len(list), list[0].Visits)
	}
}
