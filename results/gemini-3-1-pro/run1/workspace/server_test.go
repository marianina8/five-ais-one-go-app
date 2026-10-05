package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFlow(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir + "/data.json")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(store, "secret")
	server := httptest.NewServer(router)
	defer server.Close()

	// 1. Create a link
	body := bytes.NewReader([]byte(`{"url": "https://example.com/page", "alias": "my-page"}`))
	res, err := http.Post(server.URL+"/api/links", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	var created Link
	json.NewDecoder(res.Body).Decode(&created)
	if created.Code != "my-page" || created.URL != "https://example.com/page" {
		t.Fatalf("unexpected link data: %+v", created)
	}

	// 2. Alias conflict
	body = bytes.NewReader([]byte(`{"url": "https://example.com/page", "alias": "my-page"}`))
	res, _ = http.Post(server.URL+"/api/links", "application/json", body)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.StatusCode)
	}

	// 3. Create without alias
	body = bytes.NewReader([]byte(`{"url": "https://example.com/other"}`))
	res, _ = http.Post(server.URL+"/api/links", "application/json", body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	var created2 Link
	json.NewDecoder(res.Body).Decode(&created2)
	if len(created2.Code) != 7 {
		t.Fatalf("expected 7 char code, got %s", created2.Code)
	}

	// 4. Follow link
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, _ = client.Get(server.URL + "/my-page")
	if res.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", res.StatusCode)
	}
	if res.Header.Get("Location") != "https://example.com/page" {
		t.Fatalf("expected correct location")
	}

	// 5. List links (no auth)
	req, _ := http.NewRequest("GET", server.URL+"/api/links", nil)
	res, _ = client.Do(req)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}

	// 6. List links (with auth)
	req.Header.Set("Authorization", "Bearer secret")
	res, _ = client.Do(req)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var links []Link
	json.NewDecoder(res.Body).Decode(&links)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if links[0].Code != "my-page" || links[0].Visits != 1 {
		t.Fatalf("expected first link to be my-page with 1 visit, got: %+v", links[0])
	}

	// 7. Delete link
	req, _ = http.NewRequest("DELETE", server.URL+"/api/links/my-page", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res, _ = client.Do(req)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.StatusCode)
	}

	// 8. Follow deleted link
	res, _ = client.Get(server.URL + "/my-page")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.StatusCode)
	}

	// 9. Unknown method
	req, _ = http.NewRequest("PUT", server.URL+"/api/links", nil)
	res, _ = client.Do(req)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", res.StatusCode)
	}
	var errResp map[string]string
	json.NewDecoder(res.Body).Decode(&errResp)
	if errResp["error"] != "Method Not Allowed" {
		t.Fatalf("expected Method Not Allowed, got %s", errResp["error"])
	}
}

func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/data.json"
	
	s1, _ := NewStore(path)
	s1.Create("https://a.com", "link1")
	s1.Create("https://b.com", "link2")
	s1.GetAndVisit("link1")
	
	s2, _ := NewStore(path)
	links := s2.List()
	if len(links) != 2 {
		t.Fatalf("expected 2, got %d", len(links))
	}
	if links[0].Code != "link1" || links[0].Visits != 1 {
		t.Fatalf("bad data: %+v", links[0])
	}
}

func TestValidation(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewStore(tmpDir + "/data.json")
	router := NewRouter(store, "secret")
	server := httptest.NewServer(router)
	defer server.Close()

	tests := []struct{
		body string
		code int
	}{
		{`{"url": "not-a-url"}`, 400},
		{`{"url": "ftp://example.com"}`, 400},
		{`{"url": "https://example.com", "alias": "a"}`, 400}, 
		{`{"url": "https://example.com", "alias": "api"}`, 400}, 
		{`{"url": "https://example.com", "unknown": "field"}`, 400},
		{`{invalid json`, 400},
	}

	for _, tc := range tests {
		res, _ := http.Post(server.URL+"/api/links", "application/json", bytes.NewReader([]byte(tc.body)))
		if res.StatusCode != tc.code {
			t.Errorf("for body %s expected %d, got %d", tc.body, tc.code, res.StatusCode)
		}
	}
}
