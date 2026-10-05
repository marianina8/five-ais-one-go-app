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

func do(t *testing.T, h http.Handler, method, path, body string, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if admin {
		req.Header.Set("Authorization", "Bearer secret")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newSrv(t *testing.T, path string) *Server {
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}
}

func TestFlow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.json")
	s := newSrv(t, path)
	r := do(t, s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false)
	if r.Code != 201 {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := do(t, s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false); r.Code != 409 {
		t.Fatal(r.Code)
	}
	for _, b := range []string{`{"url":"ftp://x.com"}`, `{"url":"https://x.com","alias":"api"}`, `{"url":"https://x.com","alias":"a"}`, `{"url":"x"}`, `{bad`, `{"url":"https://x.com","zzz":1}`, `{"url":"http://"}`} {
		if r := do(t, s, "POST", "/api/links", b, false); r.Code != 400 {
			t.Fatal(b, r.Code)
		}
	}
	if r := do(t, s, "POST", "/api/links", `{"url":"https://x.com/`+strings.Repeat("a", 2<<20)+`"}`, false); r.Code != 413 {
		t.Fatal(r.Code)
	}
	r = do(t, s, "POST", "/api/links", `{"url":"https://example.org"}`, false)
	var l Link
	json.Unmarshal(r.Body.Bytes(), &l)
	if len(l.Code) != 7 {
		t.Fatal(l)
	}
	r = do(t, s, "GET", "/my-page", "", false)
	if r.Code != 302 || r.Header().Get("Location") != "https://example.com/x" {
		t.Fatal(r.Code, r.Header())
	}
	do(t, s, "GET", "/my-page", "", false)
	if do(t, s, "GET", "/api/links", "", false).Code != 401 {
		t.Fatal("auth")
	}
	if do(t, s, "PUT", "/api/links", "", true).Code != 405 {
		t.Fatal("405")
	}
	// restart
	s = newSrv(t, path)
	var list []Link
	json.Unmarshal(do(t, s, "GET", "/api/links", "", true).Body.Bytes(), &list)
	if len(list) != 2 || list[0].Code != "my-page" || list[0].Visits != 2 {
		t.Fatal(list)
	}
	if do(t, s, "DELETE", "/api/links/my-page", "", false).Code != 401 {
		t.Fatal("auth")
	}
	if do(t, s, "DELETE", "/api/links/my-page", "", true).Code != 204 {
		t.Fatal("del")
	}
	if do(t, s, "DELETE", "/api/links/my-page", "", true).Code != 404 {
		t.Fatal("del2")
	}
	if do(t, s, "GET", "/my-page", "", false).Code != 404 {
		t.Fatal("gone")
	}
}

func TestConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.json")
	s := newSrv(t, path)
	do(t, s, "POST", "/api/links", `{"url":"https://a.com","alias":"abc"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); do(t, s, "GET", "/abc", "", false) }()
		go func(i int) {
			defer wg.Done()
			do(t, s, "POST", "/api/links", fmt.Sprintf(`{"url":"https://a.com/%d"}`, i), false)
		}(i)
	}
	wg.Wait()
	s = newSrv(t, path)
	l := s.store.List()
	if len(l) != 31 || l[0].Visits != 30 {
		t.Fatal(len(l), l[0].Visits)
	}
}
