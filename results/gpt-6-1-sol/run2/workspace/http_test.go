package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func request(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func checkStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, status, w.Body.String())
	}
	if status != http.StatusFound && status != http.StatusNoContent {
		if w.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("wrong content type: %s", w.Header().Get("Content-Type"))
		}
		if !json.Valid(w.Body.Bytes()) {
			t.Fatalf("not JSON: %s", w.Body.String())
		}
	}
	if status >= 400 {
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if message, ok := body["error"].(string); !ok || message == "" || len(body) != 1 {
			t.Fatalf("invalid error body: %s", w.Body.String())
		}
	}
}

func TestLifecycleAndRestart(t *testing.T) {
	s := testStore(t)
	h := newHandler(s, "secret")
	w := request(h, "GET", "/api/links", "", "Bearer secret")
	checkStatus(t, w, 200)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("empty list must be an array")
	}
	w = request(h, "POST", "/api/links", `{"url":"https://example.com/page?x=1","alias":"my-page"}`, "")
	checkStatus(t, w, 201)
	var created Link
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Code != "my-page" || created.Visits != 0 || created.CreatedAt.IsZero() {
		t.Fatalf("bad link: %+v", created)
	}
	if _, err := time.Parse(time.RFC3339, created.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	checkStatus(t, request(h, "POST", "/api/links", `{"url":"https://other.example","alias":"my-page"}`, ""), 409)
	for i := 0; i < 3; i++ {
		w = request(h, "GET", "/my-page", "", "")
		checkStatus(t, w, 302)
		if w.Header().Get("Location") != created.URL {
			t.Fatal("wrong redirect target")
		}
	}
	w = request(h, "POST", "/api/links", `{"url":"http://example.com"}`, "")
	checkStatus(t, w, 201)
	var generated Link
	if err := json.Unmarshal(w.Body.Bytes(), &generated); err != nil {
		t.Fatal(err)
	}
	if len(generated.Code) != 7 || strings.ContainsAny(generated.Code, "_-") {
		t.Fatalf("invalid generated code: %q", generated.Code)
	}
	reloaded, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	h = newHandler(reloaded, "secret")
	w = request(h, "GET", "/api/links", "", "Bearer secret")
	checkStatus(t, w, 200)
	var links []Link
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].Code != "my-page" || links[0].Visits != 3 || links[1].Code != generated.Code {
		t.Fatalf("bad persisted list: %+v", links)
	}
	w = request(h, "DELETE", "/api/links/my-page", "", "Bearer secret")
	checkStatus(t, w, 204)
	if w.Body.Len() != 0 {
		t.Fatal("204 has a body")
	}
	checkStatus(t, request(h, "GET", "/my-page", "", ""), 404)
	checkStatus(t, request(h, "DELETE", "/api/links/my-page", "", "Bearer secret"), 404)
	reloaded, err = openStore(s.path)
	if err != nil || len(reloaded.List()) != 1 {
		t.Fatalf("delete did not persist: %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	h := newHandler(testStore(t), "secret")
	cases := []struct {
		name string
		body string
		status int
	}{
		{"empty", "", 400},
		{"malformed", `{`, 400},
		{"unknown", `{"url":"https://example.com","other":true}`, 400},
		{"trailing JSON", `{"url":"https://example.com"} {}`, 400},
		{"trailing junk", `{"url":"https://example.com"}x`, 400},
		{"null", `null`, 400},
		{"array", `[]`, 400},
		{"missing URL", `{}`, 400},
		{"URL type", `{"url":1}`, 400},
		{"relative", `{"url":"/page"}`, 400},
		{"no host", `{"url":"https:///page"}`, 400},
		{"empty hostname", `{"url":"https://:80/page"}`, 400},
		{"wrong scheme", `{"url":"ftp://example.com"}`, 400},
		{"bad escape", `{"url":"https://example.com/%ZZ"}`, 400},
		{"URL too long", fmt.Sprintf(`{"url":%q}`, "https://example.com/"+strings.Repeat("a", 2048)), 400},
		{"empty alias", `{"url":"https://example.com","alias":""}`, 400},
		{"null alias", `{"url":"https://example.com","alias":null}`, 400},
		{"alias type", `{"url":"https://example.com","alias":7}`, 400},
		{"short alias", `{"url":"https://example.com","alias":"ab"}`, 400},
		{"long alias", fmt.Sprintf(`{"url":"https://example.com","alias":%q}`, strings.Repeat("a", 33)), 400},
		{"reserved alias", `{"url":"https://example.com","alias":"api"}`, 400},
		{"invalid alias", `{"url":"https://example.com","alias":"a.b"}`, 400},
		{"oversized malformed", strings.Repeat("x", maxBody+1), 413},
		{"oversized valid", `{"url":"https://example.com"}` + strings.Repeat(" ", maxBody), 413},
		{"exact limit", `{"url":"https://example.com"}` + strings.Repeat(" ", maxBody-len(`{"url":"https://example.com"}`)), 201},
		{"allowed alias", `{"url":"https://example.com","alias":"A_z-9"}`, 201},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkStatus(t, request(h, "POST", "/api/links", tc.body, ""), tc.status)
		})
	}
}

func TestAuthAndRouting(t *testing.T) {
	h := newHandler(testStore(t), "secret")
	for _, path := range []string{"/api/links", "/api/links/absent"} {
		method := "GET"
		if path != "/api/links" {
			method = "DELETE"
		}
		for _, token := range []string{"", "secret", "Bearer wrong", "bearer secret", "Bearer secret "} {
			checkStatus(t, request(h, method, path, "", token), 401)
		}
	}
	for _, tc := range []struct { method, path string; status int }{
		{"PUT", "/api/links", 405},
		{"DELETE", "/api/links", 405},
		{"POST", "/abc", 405},
		{"HEAD", "/abc", 405},
		{"GET", "/api/links/abc", 405},
		{"GET", "/absent", 404},
		{"GET", "/", 404},
		{"GET", "/unknown/path", 404},
		{"GET", "/api/links/", 404},
	} {
		checkStatus(t, request(h, tc.method, tc.path, "", "Bearer secret"), tc.status)
	}
}

func TestConcurrentRequests(t *testing.T) {
	s := testStore(t)
	h := newHandler(s, "secret")
	checkStatus(t, request(h, "POST", "/api/links", `{"url":"https://example.com","alias":"shared"}`, ""), 201)
	const workers = 40
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := request(h, "POST", "/api/links", `{"url":"https://example.com"}`, "")
			if w.Code != 201 {
				t.Errorf("create: %d %s", w.Code, w.Body.String())
			}
			for j := 0; j < 3; j++ {
				w = request(h, "GET", "/shared", "", "")
				if w.Code != 302 {
					t.Errorf("visit: %d", w.Code)
				}
			}
			w = request(h, "POST", "/api/links", `{"url":"https://example.com","alias":"contested"}`, "")
			if w.Code != 201 && w.Code != 409 {
				t.Errorf("alias race: %d", w.Code)
			}
			check := request(h, "GET", "/api/links", "", "Bearer secret")
			if check.Code != 200 {
				t.Errorf("list: %d", check.Code)
			}
		}(i)
	}
	wg.Wait()
	reloaded, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	links := reloaded.List()
	if len(links) != workers+2 {
		t.Fatalf("lost links or duplicate alias: got %d", len(links))
	}
	for _, link := range links {
		if link.Code == "shared" && link.Visits != workers*3 {
			t.Fatalf("lost visits: got %d", link.Visits)
		}
	}
}

func TestStorageFailuresAndCorruption(t *testing.T) {
	s := testStore(t)
	link, err := s.Create("https://example.com", "present")
	if err != nil {
		t.Fatal(err)
	}
	original := s.path
	s.path = filepath.Join(t.TempDir(), "missing", "data.json")
	if _, err := s.Create("https://example.com", "new-link"); err == nil {
		t.Fatal("expected save failure")
	}
	if _, err := s.Visit(link.Code); err == nil {
		t.Fatal("expected visit save failure")
	}
	if err := s.Delete(link.Code); err == nil {
		t.Fatal("expected delete save failure")
	}
	if links := s.List(); len(links) != 1 || links[0] != link {
		t.Fatalf("failed changes were not rolled back: %+v", links)
	}
	h := newHandler(s, "secret")
	checkStatus(t, request(h, "GET", "/present", "", ""), 500)
	s.path = original
	if _, err := openStore(s.path); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"", "{", `[{"code":"bad","url":"ftp://example.com","created_at":"2025-01-01T00:00:00Z","visits":0}]`} {
		if err := os.WriteFile(s.path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := openStore(s.path); err == nil {
			t.Fatalf("accepted invalid data %q", data)
		}
	}
}
