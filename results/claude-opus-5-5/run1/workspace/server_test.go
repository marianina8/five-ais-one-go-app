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

func newTest(t *testing.T) (*Server, string) {
	p := filepath.Join(t.TempDir(), "data.json")
	st, err := OpenStore(p)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st, token: "secret"}, p
}

func do(h http.Handler, method, path, body string, admin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if admin {
		req.Header.Set("Authorization", "Bearer secret")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestFlow(t *testing.T) {
	s, p := newTest(t)
	rr := do(s, "POST", "/api/links", `{"url":"https://example.com/x","alias":"my-page"}`, false)
	if rr.Code != 201 {
		t.Fatalf("create %d %s", rr.Code, rr.Body)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatal("content type")
	}
	if c := do(s, "POST", "/api/links", `{"url":"https://example.com/y","alias":"my-page"}`, false).Code; c != 409 {
		t.Fatalf("dup %d", c)
	}
	rr = do(s, "POST", "/api/links", `{"url":"http://a.com"}`, false)
	var l Link
	json.Unmarshal(rr.Body.Bytes(), &l)
	if rr.Code != 201 || len(l.Code) != 7 {
		t.Fatalf("gen %d %s", rr.Code, rr.Body)
	}
	rr = do(s, "GET", "/my-page", "", false)
	if rr.Code != 302 || rr.Header().Get("Location") != "https://example.com/x" {
		t.Fatalf("redirect %d", rr.Code)
	}
	if c := do(s, "GET", "/api/links", "", false).Code; c != 401 {
		t.Fatalf("auth %d", c)
	}
	rr = do(s, "GET", "/api/links", "", true)
	var list []Link
	json.Unmarshal(rr.Body.Bytes(), &list)
	if len(list) != 2 || list[0].Code != "my-page" || list[0].Visits != 1 {
		t.Fatalf("list %s", rr.Body)
	}
	// restart
	st2, err := OpenStore(p)
	if err != nil || len(st2.List()) != 2 || st2.List()[0].Visits != 1 {
		t.Fatalf("restart %v", err)
	}
	if c := do(s, "DELETE", "/api/links/my-page", "", false).Code; c != 401 {
		t.Fatalf("del auth %d", c)
	}
	if c := do(s, "DELETE", "/api/links/my-page", "", true).Code; c != 204 {
		t.Fatalf("del %d", c)
	}
	if c := do(s, "DELETE", "/api/links/my-page", "", true).Code; c != 404 {
		t.Fatalf("del2 %d", c)
	}
	if c := do(s, "GET", "/my-page", "", false).Code; c != 404 {
		t.Fatalf("after del %d", c)
	}
	if c := do(s, "PUT", "/api/links", "", true).Code; c != 405 {
		t.Fatalf("405 %d", c)
	}
}

func TestValidation(t *testing.T) {
	s, _ := newTest(t)
	bad := []string{
		`{"url":"ftp://a.com"}`, `{"url":"/rel"}`, `{"url":"http://"}`, `{}`,
		`{"url":"http://a.com","alias":"ab"}`, `{"url":"http://a.com","alias":"api"}`,
		`{"url":"http://a.com","alias":"a b c"}`, `{"url":"http://a.com","x":1}`, `{bad`,
		`{"url":"http://a.com/` + strings.Repeat("a", 2048) + `"}`,
	}
	for _, b := range bad {
		if c := do(s, "POST", "/api/links", b, false).Code; c != 400 {
			t.Errorf("%s -> %d", b, c)
		}
	}
	big := `{"url":"http://a.com","alias":"` + strings.Repeat("a", 2<<20) + `"}`
	if c := do(s, "POST", "/api/links", big, false).Code; c != 413 {
		t.Errorf("big -> %d", c)
	}
}

func TestConcurrent(t *testing.T) {
	s, p := newTest(t)
	do(s, "POST", "/api/links", `{"url":"http://a.com","alias":"hot"}`, false)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); do(s, "POST", "/api/links", `{"url":"http://b.com"}`, false) }()
		go func() { defer wg.Done(); do(s, "GET", "/hot", "", false) }()
	}
	wg.Wait()
	st2, _ := OpenStore(p)
	list := st2.List()
	if len(list) != 51 || list[0].Visits != 50 {
		t.Fatalf("got %d links, %d visits", len(list), list[0].Visits)
	}
}
