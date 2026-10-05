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
