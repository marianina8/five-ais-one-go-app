package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestListOrderAndIsolation(t *testing.T) {
	s, _, path := fixture(t)
	// Deliberately put newer records first in the file, with a timestamp tie.
	stamp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	links := []Link{
		{Code: "new", URL: "https://example.com", CreatedAt: stamp.Add(time.Hour)},
		{Code: "bbb", URL: "https://example.com", CreatedAt: stamp},
		{Code: "aaa", URL: "https://example.com", CreatedAt: stamp},
	}
	data, err := json.Marshal(links)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s.List()
	if got[0].Code != "aaa" || got[1].Code != "bbb" || got[2].Code != "new" {
		t.Fatalf("not oldest first: %+v", got)
	}
	got[0].URL = "https://changed.example"
	if s.List()[0].URL != "https://example.com" {
		t.Fatal("list exposed mutable internal state")
	}
}

func TestFailedSaveDoesNotCommit(t *testing.T) {
	s, h, path := fixture(t)
	request(t, h, "POST", "/api/links", `{"url":"https://example.com","alias":"original"}`, "", 201)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A missing parent directory causes writes to fail even when run as root.
	s.path = filepath.Join(t.TempDir(), "missing", "data.json")
	request(t, h, "POST", "/api/links", `{"url":"https://example.com","alias":"failed"}`, "", 500)
	request(t, h, "GET", "/original", "", "", 500)
	request(t, h, "DELETE", "/api/links/original", "", "Bearer secret", 500)
	links := s.List()
	if len(links) != 1 || links[0].Code != "original" || links[0].Visits != 0 {
		t.Fatalf("failed write changed state: %+v", links)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("previous snapshot changed: %v", err)
	}
	s.path = path
	request(t, h, "GET", "/original", "", "", 302)
}

func TestInvalidStorage(t *testing.T) {
	for _, data := range []string{
		``, `{`, `{}`, `null`, `[] []`,
		`[{"code":"abc","url":"https://example.com","created_at":"bad","visits":0}]`,
		`[{"code":"abc","url":"/relative","created_at":"2025-01-01T00:00:00Z","visits":0}]`,
		`[{"code":"api","url":"http://example.com","created_at":"2025-01-01T00:00:00Z","visits":0}]`,
		`[{"code":"abc","url":"http://example.com","visits":0}]`,
		`[{"code":"abc","url":"http://example.com","created_at":"2025-01-01T00:00:00Z","visits":-1}]`,
		`[{"code":"abc","url":"http://example.com","created_at":"2025-01-01T00:00:00Z"},{"code":"abc","url":"http://other.com","created_at":"2025-01-01T00:00:00Z"}]`,
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "data.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenStore(path); err == nil {
				t.Fatal("accepted corrupt storage")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != data {
				t.Fatal("opening corrupt storage must not overwrite it")
			}
		})
	}
}

func TestSnapshotsAreAlwaysComplete(t *testing.T) {
	s, _, path := fixture(t)
	alias := "counter"
	if _, err := s.Create("http://example.com", &alias); err != nil {
		t.Fatal(err)
	}
	// A reader opening the file during saves must never see truncated JSON.
	stop := make(chan struct{})
	fail := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err == nil {
				var links []Link
				err = json.Unmarshal(data, &links)
				if err == nil && (len(links) != 1 || links[0].Code != alias) {
					err = errors.New("incomplete snapshot")
				}
			}
			if err != nil {
				fail <- err
				return
			}
		}
	}()
	for i := 0; i < 100; i++ {
		if _, err := s.Follow(alias); err != nil {
			t.Error(err)
			break
		}
	}
	close(stop)
	wg.Wait()
	select {
	case err := <-fail:
		t.Fatal(err)
	default:
	}
	leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".shortener-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v, %v", leftovers, err)
	}
}

func TestStoreErrors(t *testing.T) {
	s, _, _ := fixture(t)
	if _, err := s.Follow("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.Create("relative", nil); err == nil {
		t.Fatal("accepted invalid URL")
	}
	bad := "api"
	if _, err := s.Create("https://example.com", &bad); err == nil {
		t.Fatal("accepted reserved alias")
	}
	if _, err := OpenStore(t.TempDir()); err == nil {
		t.Fatal("accepted a directory as the data file")
	}
}
