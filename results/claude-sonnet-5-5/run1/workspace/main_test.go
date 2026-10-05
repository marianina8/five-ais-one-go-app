package main

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func setup(t *testing.T) (*Server, string) {
	p := filepath.Join(t.TempDir(), "d.json")
	st, err := OpenStore(p)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}, p
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
	s, p := setup(t)
	w := do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false).Code != 409 {
		t.Fatal("want 409")
	}
	for _, b := range []string{`{"url":"ftp://x.com"}`, `{"url":"https://a.com","alias":"api"}`, `{"url":"https://a.com","alias":"a"}`, `{"url":"https://a.com","x":1}`, `{`, `{"url":"/rel"}`} {
		if c := do(s, "POST", "/api/links", b, false).Code; c != 400 {
			t.Fatal(b, c)
		}
	}
	if c := do(s, "POST", "/api/links", `{"url":"https://a.com/`+strings.Repeat("a", 2<<20)+`"}`, false).Code; c != 413 {
		t.Fatal(c)
	}
	w = do(s, "GET", "/my-page", "", false)
	if w.Code != 302 || w.Header().Get("Location") != "https://example.com/x" {
		t.Fatal(w.Code)
	}
	if do(s, "GET", "/nope", "", false).Code != 404 {
		t.Fatal()
	}
	if do(s, "GET", "/api/links", "", false).Code != 401 {
		t.Fatal()
	}
	if do(s, "PUT", "/api/links", "", false).Code != 405 {
		t.Fatal()
	}
	// restart
	st2, err := OpenStore(p)
	if err != nil {
		t.Fatal(err)
	}
	s2 := &Server{store: st2, token: "secret"}
	w = do(s2, "GET", "/api/links", "", true)
	var ls []Link
	json.Unmarshal(w.Body.Bytes(), &ls)
	if len(ls) != 1 || ls[0].Visits != 1 {
		t.Fatal(w.Body.String())
	}
	if do(s2, "DELETE", "/api/links/my-page", "", true).Code != 204 {
		t.Fatal()
	}
	if do(s2, "GET", "/my-page", "", false).Code != 404 {
		t.Fatal()
	}
	if do(s2, "DELETE", "/api/links/my-page", "", true).Code != 404 {
		t.Fatal()
	}
}

func TestConcurrent(t *testing.T) {
	s, _ := setup(t)
	do(s, "POST", "/api/links", `{"url":"https://e.com","alias":"abc"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); do(s, "GET", "/abc", "", false) }()
		go func() {
			defer wg.Done()
			if do(s, "POST", "/api/links", `{"url":"https://e.com"}`, false).Code != 201 {
				t.Error("create")
			}
		}()
	}
	wg.Wait()
	ls := s.store.List()
	if len(ls) != 41 || ls[0].Visits != 40 {
		t.Fatal(len(ls), ls[0].Visits)
	}
}
