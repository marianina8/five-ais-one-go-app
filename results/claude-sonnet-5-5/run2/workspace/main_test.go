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

func setup(t *testing.T, path string) (*Server, *Store) {
	t.Helper()
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}, st
}

func do(s *Server, method, path, body, auth string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth != "" {
		r.Header.Set("Authorization", "Bearer "+auth)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestFlow(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	s, _ := setup(t, p)
	w := do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body)
	}
	if w = do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	w = do(s, "POST", "/api/links", `{"url":"http://a.b"}`, "")
	var l Link
	json.Unmarshal(w.Body.Bytes(), &l)
	if w.Code != 201 || len(l.Code) != 7 {
		t.Fatal(w.Code, w.Body)
	}
	for _, b := range []string{`{"url":"ftp://x.com"}`, `{"url":"/rel"}`, `{"url":"https://a.com","alias":"api"}`,
		`{"url":"https://a.com","alias":"ab"}`, `{"url":"https://a.com","x":1}`, `{bad`, `{"url":"https://a.com"} x`, `{}`, `{"url":"http://"}`} {
		if w = do(s, "POST", "/api/links", b, ""); w.Code != 400 {
			t.Fatal(b, w.Code)
		}
	}
	if w = do(s, "POST", "/api/links", `{"url":"https://a.com/`+strings.Repeat("a", 2<<20)+`"}`, ""); w.Code != 413 {
		t.Fatal(w.Code)
	}
	w = do(s, "GET", "/my-page", "", "")
	if w.Code != 302 || w.Header().Get("Location") != "https://example.com/x" {
		t.Fatal(w.Code)
	}
	if do(s, "GET", "/nope", "", "").Code != 404 {
		t.Fatal("404")
	}
	if do(s, "GET", "/api/links", "", "").Code != 401 || do(s, "GET", "/api/links", "", "bad").Code != 401 {
		t.Fatal("401")
	}
	w = do(s, "GET", "/api/links", "", "secret")
	var ls []Link
	json.Unmarshal(w.Body.Bytes(), &ls)
	if w.Code != 200 || len(ls) != 2 || ls[0].Code != "my-page" || ls[0].Visits != 1 {
		t.Fatal(w.Code, w.Body)
	}
	// restart
	s2, _ := setup(t, p)
	ls = s2.store.List()
	if len(ls) != 2 || ls[0].Visits != 1 {
		t.Fatal(ls)
	}
	if do(s2, "PUT", "/my-page", "", "").Code != 405 || do(s2, "PUT", "/api/links", "", "").Code != 405 ||
		do(s2, "POST", "/api/links/my-page", "", "secret").Code != 405 {
		t.Fatal("405")
	}
	if do(s2, "DELETE", "/api/links/my-page", "", "").Code != 401 {
		t.Fatal("401 del")
	}
	if do(s2, "DELETE", "/api/links/my-page", "", "secret").Code != 204 {
		t.Fatal("del")
	}
	if do(s2, "DELETE", "/api/links/my-page", "", "secret").Code != 404 || do(s2, "GET", "/my-page", "", "").Code != 404 {
		t.Fatal("after del")
	}
}

func TestConcurrent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.json")
	s, st := setup(t, p)
	do(s, "POST", "/api/links", `{"url":"https://a.com","alias":"abc"}`, "")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			do(s, "GET", "/abc", "", "")
			do(s, "POST", "/api/links", `{"url":"https://a.com"}`, "")
		}()
	}
	wg.Wait()
	ls := st.List()
	if len(ls) != 51 || ls[0].Visits != 50 {
		t.Fatal(len(ls), ls[0].Visits)
	}
	s2, _ := setup(t, p)
	ls = s2.store.List()
	if len(ls) != 51 || ls[0].Visits != 50 {
		t.Fatal(fmt.Sprint(len(ls)))
	}
}

var _ = http.StatusOK
