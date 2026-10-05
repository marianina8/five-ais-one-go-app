package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAtomicFileReplacement(t *testing.T) {
	s := testStore(t)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(s.path)
			if err != nil {
				t.Errorf("data file disappeared: %v", err)
				return
			}
			var links []Link
			if err := json.Unmarshal(data, &links); err != nil {
				t.Errorf("partial or corrupt JSON observed: %v", err)
				return
			}
		}
	}()
	for i := 0; i < 50; i++ {
		code := fmt.Sprintf("code-%d", i)
		if _, err := s.Create("https://example.com", code); err != nil {
			t.Error(err)
			break
		}
		if _, err := s.Visit(code); err != nil {
			t.Error(err)
			break
		}
		if err := s.Delete(code); err != nil {
			t.Error(err)
			break
		}
	}
	close(stop)
	<-done
}

func TestConcurrentDeletes(t *testing.T) {
	s := testStore(t)
	h := newHandler(s, "secret")
	checkStatus(t, request(h, "POST", "/api/links", `{"url":"https://example.com","alias":"remove-me"}`, ""), 201)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request(h, "DELETE", "/api/links/remove-me", "", "Bearer secret")
			switch w.Code {
			case 204:
				successes.Add(1)
			case 404:
			default:
				t.Errorf("delete returned %d", w.Code)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("got %d successful deletes, want exactly one", successes.Load())
	}
	checkStatus(t, request(h, "GET", "/remove-me", "", ""), 404)
	reloaded, err := openStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.List()) != 0 {
		t.Fatal("concurrent delete did not persist")
	}
}

func TestRequiresAdminToken(t *testing.T) {
	if os.Getenv("SHORTENER_TEST_MAIN") == "1" {
		main()
		return
	}
	for _, empty := range []bool{false, true} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRequiresAdminToken$")
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "ADMIN_TOKEN=") && !strings.HasPrefix(entry, "SHORTENER_TEST_MAIN=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "SHORTENER_TEST_MAIN=1")
		if empty {
			cmd.Env = append(cmd.Env, "ADMIN_TOKEN=")
		}
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("missing ADMIN_TOKEN did not produce a non-zero exit")
		}
		if !strings.Contains(string(output), "ADMIN_TOKEN must be set and non-empty") {
			t.Fatalf("missing startup error: %s", output)
		}
	}
}
