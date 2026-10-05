package main

import (
	"bytes"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainStartupErrors(t *testing.T) {
	if os.Getenv("SHORTENER_TEST_CHILD") == "1" {
		flag.CommandLine = flag.NewFlagSet("shortener", flag.ExitOnError)
		os.Args = []string{"shortener", "-data", os.Getenv("SHORTENER_TEST_DATA")}
		main()
		t.Fatal("main returned instead of exiting")
	}
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, message string
	}{
		{"empty admin token", "", "ADMIN_TOKEN"},
		{"corrupt storage", "secret", "open storage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMainStartupErrors$")
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "ADMIN_TOKEN=") && !strings.HasPrefix(entry, "SHORTENER_TEST_") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "SHORTENER_TEST_CHILD=1", "ADMIN_TOKEN="+tc.token, "SHORTENER_TEST_DATA="+path)
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() == 0 {
				t.Fatalf("expected non-zero exit, got %v; output: %s", err, output)
			}
			if !bytes.Contains(output, []byte(tc.message)) {
				t.Fatalf("missing startup error: %s", output)
			}
		})
	}
}

func TestUnicodeURLLength(t *testing.T) {
	_, h, _ := fixture(t)
	prefix := "https://example.com/"
	value := prefix + strings.Repeat("é", 2048-len(prefix))
	request(t, h, "POST", "/api/links", `{"url":"`+value+`"}`, "", 201)
	request(t, h, "POST", "/api/links", `{"url":"`+value+`é"}`, "", 400)
}

func TestHTTPServerRedirect(t *testing.T) {
	s, h, path := fixture(t)
	alias := "redirect"
	if _, err := s.Create("https://example.com/page?x=1", &alias); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := client.Get(server.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 302 || response.Header.Get("Location") != "https://example.com/page?x=1" {
		t.Fatalf("bad redirect: status=%d headers=%v", response.StatusCode, response.Header)
	}
	reloaded, err := OpenStore(path)
	if err != nil || reloaded.List()[0].Visits != 1 {
		t.Fatalf("redirect did not persist visit: %v", err)
	}
}
