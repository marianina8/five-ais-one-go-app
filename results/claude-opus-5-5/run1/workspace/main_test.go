package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestServer(t *testing.T, path string) *Server {
	t.Helper()
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}
}

func do(s http.Handler, method, path, body string, admin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if admin {
		req.Header.Set("Authorization", "Bearer secret")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestFlow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	s := newTestServer(t, path)

	rec := do(s, "POST", "/api/links", `{"url":"https://example.com/a","alias":"my-page"}`, false)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("ct %q", ct)
	}
	if rec := do(s, "POST", "/api/links", `{"url":"https://example.com/b","alias":"my-page"}`, false); rec.Code != 409 {
		t.Fatalf("dup: %d", rec.Code)
	}
	rec = do(s, "POST", "/api/links", `{"url":"http://example.com/c"}`, false)
	if rec.Code != 201 {
		t.Fatalf("create2: %d", rec.Code)
	}
	var l Link
	json.Unmarshal(rec.Body.Bytes(), &l)
	if len(l.Code) != 7 {
		t.Fatalf("code %q", l.Code)
	}

	for _, b := range []string{
		`{"url":"ftp://x.com"}`, `{"url":"/rel"}`, `{"url":"http://"}`, `{}`, `{"url":"https://x.com","x":1}`,
		`{"url":"https://x.com","alias":"api"}`, `{"url":"https://x.com","alias":"ab"}`, `{bad`,
		`{"url":"https://x.com/` + strings.Repeat("a", 2048) + `"}`,
	} {
		if rec := do(s, "POST", "/api/links", b, false); rec.Code != 400 {
			t.Errorf("%s: %d", b, rec.Code)
		}
	}
	big := `{"url":"https://x.com","alias":"` + strings.Repeat("a", 2<<20) + `"}`
	if rec := do(s, "POST", "/api/links", big, false); rec.Code != 413 {
		t.Errorf("big: %d", rec.Code)
	}

	rec = do(s, "GET", "/my-page", "", false)
	if rec.Code != 302 || rec.Header().Get("Location") != "https://example.com/a" {
		t.Fatalf("redirect: %d", rec.Code)
	}
	if rec := do(s, "GET", "/nope", "", false); rec.Code != 404 {
		t.Fatalf("404: %d", rec.Code)
	}
	if rec := do(s, "GET", "/api/links", "", false); rec.Code != 401 {
		t.Fatalf("401: %d", rec.Code)
	}
	if rec := do(s, "PUT", "/api/links", "", false); rec.Code != 405 {
		t.Fatalf("405: %d", rec.Code)
	}
	rec = do(s, "GET", "/api/links", "", true)
	var list []Link
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 2 || list[0].Code != "my-page" || list[0].Visits != 1 {
		t.Fatalf("list: %+v", list)
	}

	// restart
	s2 := newTestServer(t, path)
	if got, _ := s2.store.Get("my-page"); got.Visits != 1 {
		t.Fatalf("persist: %+v", got)
	}
	if rec := do(s2, "DELETE", "/api/links/my-page", "", false); rec.Code != 401 {
		t.Fatalf("del401: %d", rec.Code)
	}
	if rec := do(s2, "DELETE", "/api/links/my-page", "", true); rec.Code != 204 {
		t.Fatalf("del: %d", rec.Code)
	}
	if rec := do(s2, "DELETE", "/api/links/my-page", "", true); rec.Code != 404 {
		t.Fatalf("del404: %d", rec.Code)
	}
	if rec := do(s2, "GET", "/my-page", "", false); rec.Code != 404 {
		t.Fatalf("after del: %d", rec.Code)
	}
}

func TestConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	s := newTestServer(t, path)
	do(s, "POST", "/api/links", `{"url":"https://example.com","alias":"hot"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); do(s, "POST", "/api/links", `{"url":"https://example.com"}`, false) }()
		go func() { defer wg.Done(); do(s, "GET", "/hot", "", false) }()
	}
	wg.Wait()
	s2 := newTestServer(t, path)
	list := s2.store.List()
	if len(list) != 51 {
		t.Fatalf("links: %d", len(list))
	}
	if l, _ := s2.store.Get("hot"); l.Visits != 50 {
		t.Fatalf("visits: %d", l.Visits)
	}
}
