package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestURLShortener(t *testing.T) {
	// Setup
	tmpFile, err := os.CreateTemp("", "data*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		store:      store,
		adminToken: "secret",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", server.handleCreateLink)
	mux.HandleFunc("GET /api/links", server.handleListLinks)
	mux.HandleFunc("DELETE /api/links/{code}", server.handleDeleteLink)
	mux.HandleFunc("GET /{code}", server.handleFollowLink)

	ts := httptest.NewServer(wrapInterceptor(mux))
	defer ts.Close()

	client := ts.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // do not follow redirects
	}

	// 1. Create a link
	reqBody := `{"url": "https://example.com/page", "alias": "my-page"}`
	req, _ := http.NewRequest("POST", ts.URL+"/api/links", bytes.NewBufferString(reqBody))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var link Link
	json.NewDecoder(resp.Body).Decode(&link)
	resp.Body.Close()

	if link.Code != "my-page" || link.URL != "https://example.com/page" || link.Visits != 0 {
		t.Errorf("unexpected link: %+v", link)
	}

	// 2. Follow the link
	req, _ = http.NewRequest("GET", ts.URL+"/my-page", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://example.com/page" {
		t.Errorf("expected redirect to https://example.com/page, got %s", loc)
	}
	resp.Body.Close()

	// 3. List links (Admin)
	req, _ = http.NewRequest("GET", ts.URL+"/api/links", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var links []Link
	json.NewDecoder(resp.Body).Decode(&links)
	resp.Body.Close()

	if len(links) != 1 || links[0].Visits != 1 {
		t.Errorf("expected 1 link with 1 visit, got %+v", links)
	}

	// 4. Delete the link
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/links/my-page", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	// 5. Follow the link again (should be 404)
	req, _ = http.NewRequest("GET", ts.URL+"/my-page", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. Test 405 Method Not Allowed
	req, _ = http.NewRequest("POST", ts.URL+"/api/links/foo", nil) // POST on DELETE path
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected JSON 405 response, got %s", ct)
	}
	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp["error"] == "" {
		t.Errorf("expected error message in 405 response")
	}
}

func TestStoreConcurrency(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "data*.json")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, _ := NewStore(tmpFile.Name())
	_, _ = store.Create("race", "http://example.com")

	start := make(chan struct{})
	done := make(chan struct{})

	for i := 0; i < 100; i++ {
		go func() {
			<-start
			store.Visit("race")
			done <- struct{}{}
		}()
	}

	close(start)
	for i := 0; i < 100; i++ {
		<-done
	}

	l, _ := store.GetByCode("race")
	if l.Visits != 100 {
		t.Errorf("expected 100 visits, got %d", l.Visits)
	}

	// Verify file state
	store2, _ := NewStore(tmpFile.Name())
	l2, _ := store2.GetByCode("race")
	if l2.Visits != 100 {
		t.Errorf("expected 100 visits in reloaded store, got %d", l2.Visits)
	}
}
