package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) (*Store, http.Handler, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, NewHandler(s, "secret"), path
}

func request(t *testing.T, h http.Handler, method, path, body, token string, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, path, w.Code, status, w.Body.String())
	}
	if status != http.StatusFound && status != http.StatusNoContent {
		if w.Header().Get("Content-Type") != "application/json" || !json.Valid(w.Body.Bytes()) {
			t.Fatalf("response is not JSON: headers=%v body=%s", w.Header(), w.Body.String())
		}
	}
	if status >= 400 {
		var value map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil || len(value) != 1 {
			t.Fatalf("invalid error object: %s", w.Body.String())
		}
		if message, ok := value["error"].(string); !ok || message == "" {
			t.Fatalf("missing error message: %s", w.Body.String())
		}
	}
	return w
}

func decodeLink(t *testing.T, w *httptest.ResponseRecorder) Link {
	t.Helper()
	var link Link
	if err := json.Unmarshal(w.Body.Bytes(), &link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestLifecycleAndRestart(t *testing.T) {
	_, h, path := fixture(t)
	w := request(t, h, "POST", "/api/links", `{"url":"https://example.com/page?x=1#part","alias":"my-page"}`, "", 201)
	link := decodeLink(t, w)
	if link.Code != "my-page" || link.URL != "https://example.com/page?x=1#part" || link.Visits != 0 || link.CreatedAt.IsZero() {
		t.Fatalf("unexpected link: %+v", link)
	}
	request(t, h, "POST", "/api/links", `{"url":"http://example.net","alias":"my-page"}`, "", 409)

	// Each operation must already be visible in a freshly loaded store.
	reloaded, err := OpenStore(path)
	if err != nil || len(reloaded.List()) != 1 {
		t.Fatalf("create was not persisted: %v", err)
	}
	h = NewHandler(reloaded, "secret")
	for i := 0; i < 3; i++ {
		w = request(t, h, "GET", "/my-page", "", "", 302)
		if w.Header().Get("Location") != link.URL {
			t.Fatalf("wrong Location: %v", w.Header())
		}
		saved, err := OpenStore(path)
		if err != nil || saved.List()[0].Visits != uint64(i+1) {
			t.Fatalf("visit not persisted: %v", err)
		}
	}
	w = request(t, h, "GET", "/api/links", "", "Bearer secret", 200)
	var links []Link
	if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil || len(links) != 1 || links[0].Visits != 3 {
		t.Fatalf("wrong list: %s", w.Body.String())
	}
	w = request(t, h, "DELETE", "/api/links/my-page", "", "Bearer secret", 204)
	if w.Body.Len() != 0 {
		t.Fatal("204 must have no body")
	}
	request(t, h, "GET", "/my-page", "", "", 404)
	request(t, h, "DELETE", "/api/links/my-page", "", "Bearer secret", 404)
	reloaded, err = OpenStore(path)
	if err != nil || len(reloaded.List()) != 0 {
		t.Fatalf("delete was not persisted: %v", err)
	}
	request(t, NewHandler(reloaded, "secret"), "GET", "/my-page", "", "", 404)
	w = request(t, h, "GET", "/api/links", "", "Bearer secret", 200)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty list is not array: %s", w.Body.String())
	}
	// Deleted aliases are reusable.
	request(t, h, "POST", "/api/links", `{"url":"http://example.net","alias":"my-page"}`, "", 201)
}

func TestCreateValidation(t *testing.T) {
	_, h, _ := fixture(t)
	bad := []string{
		``, `{`, `[]`, `null`, `{}`, `{"url":null}`, `{"url":7}`,
		`{"url":"https://example.com","unknown":true}`,
		`{"url":"https://example.com"} {}`,
		`{"url":"/relative"}`, `{"url":"ftp://example.com"}`,
		`{"url":"https:///path"}`, `{"url":"http://"}`,
		`{"url":"https://exa mple.com"}`, `{"url":"https://example.com\n"}`,
		`{"url":"https://example.com","alias":"api"}`,
		`{"url":"https://example.com","alias":"ab"}`,
		`{"url":"https://example.com","alias":"has space"}`,
		`{"url":"https://example.com","alias":"a/b"}`,
		`{"url":"https://example.com","alias":""}`,
		`{"url":"https://example.com","alias":null}`,
		`{"url":"https://example.com","alias":123}`,
		fmt.Sprintf(`{"url":"https://example.com","alias":%q}`, strings.Repeat("x", 33)),
		fmt.Sprintf(`{"url":%q}`, "https://example.com/"+strings.Repeat("a", 2048)),
	}
	for _, body := range bad {
		t.Run(body, func(t *testing.T) {
			request(t, h, "POST", "/api/links", body, "", 400)
		})
	}
	for _, alias := range []string{"abc", "API", "Ab_09-", strings.Repeat("x", 32)} {
		request(t, h, "POST", "/api/links", fmt.Sprintf(`{"url":"http://localhost:8080/path","alias":%q}`, alias), "", 201)
	}
	body := fmt.Sprintf(`{"url":%q}`, "https://example.com/"+strings.Repeat("a", 2048-len("https://example.com/")))
	request(t, h, "POST", "/api/links", body, "", 201)
}

func TestBodyLimit(t *testing.T) {
	_, h, _ := fixture(t)
	body := `{"url":"https://example.com"}`
	request(t, h, "POST", "/api/links", body+strings.Repeat(" ", maxBody-len(body)), "", 201)
	request(t, h, "POST", "/api/links", body+strings.Repeat(" ", maxBody+1-len(body)), "", 413)
	request(t, h, "POST", "/api/links", strings.Repeat("x", maxBody+1), "", 413)
}

func TestAuthAndRouting(t *testing.T) {
	_, h, _ := fixture(t)
	for _, token := range []string{"", "secret", "Bearer wrong", "bearer secret", "Bearer secret "} {
		request(t, h, "GET", "/api/links", "", token, 401)
		request(t, h, "DELETE", "/api/links/missing", "", token, 401)
	}
	request(t, h, "GET", "/api/links", "", "Bearer secret", 200)
	request(t, h, "GET", "/missing", "", "", 404)
	for _, path := range []string{"/", "/api/unknown", "/api/links/", "/api/links/one/two", "/one/two"} {
		request(t, h, "GET", path, "", "", 404)
	}
	for _, tc := range []struct{ method, path, allow string }{
		{"PUT", "/api/links", "GET, POST"},
		{"DELETE", "/api/links", "GET, POST"},
		{"GET", "/api/links/abc", "DELETE"},
		{"POST", "/abc", "GET"},
		{"HEAD", "/abc", "GET"},
		{"OPTIONS", "/api/links", "GET, POST"},
	} {
		w := request(t, h, tc.method, tc.path, "", "", 405)
		if w.Header().Get("Allow") != tc.allow {
			t.Errorf("wrong Allow header: %v", w.Header())
		}
	}
}

func TestConcurrentRequests(t *testing.T) {
	s, h, path := fixture(t)
	request(t, h, "POST", "/api/links", `{"url":"https://example.com","alias":"shared"}`, "", 201)
	var wg sync.WaitGroup
	const n = 60
	errors := make(chan string, n*3)
	codes := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for _, op := range []struct {
				method, path, body, token string
				status                    int
			}{
				{"POST", "/api/links", `{"url":"https://example.net"}`, "", 201},
				{"GET", "/shared", "", "", 302},
				{"GET", "/api/links", "", "Bearer secret", 200},
			} {
				w := httptest.NewRecorder()
				r := httptest.NewRequest(op.method, op.path, strings.NewReader(op.body))
				r.Header.Set("Authorization", op.token)
				h.ServeHTTP(w, r)
				if w.Code != op.status {
					errors <- fmt.Sprintf("worker %d: status %d: %s", i, w.Code, w.Body.String())
				}
				if op.method == "POST" {
					var link Link
					if err := json.Unmarshal(w.Body.Bytes(), &link); err != nil {
						errors <- err.Error()
					} else {
						codes <- link.Code
					}
				}
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	close(codes)
	for err := range errors {
		t.Error(err)
	}
	seen := make(map[string]bool)
	pattern := regexp.MustCompile(`^[A-Za-z0-9]{7}$`)
	for code := range codes {
		if seen[code] || !pattern.MatchString(code) {
			t.Errorf("invalid or duplicated random code %q", code)
		}
		seen[code] = true
	}
	if len(seen) != n || len(s.List()) != n+1 {
		t.Fatal("lost links")
	}
	reloaded, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.List()) != n+1 {
		t.Fatal("lost persisted links")
	}
	for _, link := range reloaded.List() {
		if link.Code == "shared" && link.Visits != n {
			t.Fatalf("lost visits: %d", link.Visits)
		}
	}
}

func TestConcurrentAliasConflict(t *testing.T) {
	_, h, _ := fixture(t)
	const n = 20
	results := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/api/links", strings.NewReader(`{"url":"http://example.com","alias":"same"}`)))
			results <- w.Code
		}()
	}
	wg.Wait()
	close(results)
	created := 0
	for status := range results {
		if status == 201 {
			created++
		} else if status != 409 {
			t.Errorf("unexpected status %d", status)
		}
	}
	if created != 1 {
		t.Fatalf("got %d creations, want 1", created)
	}
}
