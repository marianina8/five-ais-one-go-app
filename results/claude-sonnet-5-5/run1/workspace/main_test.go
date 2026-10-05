package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func setup(t *testing.T, path string) (*Server, *Store) {
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}, st
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
	s, _ := setup(t, p)
	w := do(s, "POST", "/api/links", `{"url":"https://example.com/a","alias":"my-page"}`, false)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if do(s, "POST", "/api/links", `{"url":"https://example.com/a","alias":"my-page"}`, false).Code != 409 {
		t.Fatal("want 409")
	}
	for _, b := range []string{`{"url":"ftp://x.com"}`, `{"url":"https://a.com","alias":"api"}`, `{"url":"https://a.com","alias":"a"}`, `{bad`, `{"url":"https://a.com","x":1}`, `{"url":"/rel"}`} {
		if c := do(s, "POST", "/api/links", b, false).Code; c != 400 {
			t.Fatal(b, c)
		}
	}
	if c := do(s, "POST", "/api/links", `{"url":"https://a.com/`+strings.Repeat("a", 1<<20)+`"}`, false).Code; c != 413 {
		t.Fatal(c)
	}
	w = do(s, "GET", "/my-page", "", false)
	if w.Code != 302 || w.Header().Get("Location") != "https://example.com/a" {
		t.Fatal(w.Code)
	}
	if do(s, "GET", "/nope", "", false).Code != 404 {
		t.Fatal("404")
	}
	if do(s, "GET", "/api/links", "", false).Code != 401 {
		t.Fatal("401")
	}
	if do(s, "PUT", "/api/links", "", false).Code != 405 {
		t.Fatal("405")
	}
	// restart
	s2, _ := setup(t, p)
	w = do(s2, "GET", "/api/links", "", true)
	var ls []Link
	json.Unmarshal(w.Body.Bytes(), &ls)
	if len(ls) != 1 || ls[0].Visits != 1 {
		t.Fatal(ls)
	}
	if do(s2, "DELETE", "/api/links/my-page", "", true).Code != 204 {
		t.Fatal("del")
	}
	if do(s2, "DELETE", "/api/links/my-page", "", true).Code != 404 {
		t.Fatal("del404")
	}
	if do(s2, "GET", "/my-page", "", false).Code != 404 {
		t.Fatal("gone")
	}
}

func TestConcurrent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	s, _ := setup(t, p)
	do(s, "POST", "/api/links", `{"url":"https://e.com","alias":"abc"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); do(s, "GET", "/abc", "", false) }()
		go func() { defer wg.Done(); do(s, "POST", "/api/links", `{"url":"https://e.com"}`, false) }()
	}
	wg.Wait()
	s2, _ := setup(t, p)
	ls := s2.store.List()
	if len(ls) != 51 || ls[0].Visits != 50 {
		t.Fatal(len(ls), ls[0].Visits)
	}
}
