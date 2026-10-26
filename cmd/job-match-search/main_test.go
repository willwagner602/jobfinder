package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jobfinder/internal/db"
	"jobfinder/internal/matching"
)

func TestMainSuccessPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.sqlite")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	oldArgs := os.Args
	os.Args = []string{"job-match-search"}
	t.Setenv("JOB_DB_PATH", path)
	t.Setenv("JOB_MATCH_LLM_URL", "https://llm.example.test")
	t.Setenv("JOB_MATCH_LLM_MODEL", "m")
	t.Setenv("JOB_MATCH_LLM_API_KEY", "k")
	defer func() { os.Args = oldArgs }()
	main()
}

func TestClassifiersFromEnv(t *testing.T) {
	env := map[string]string{
		"JOB_MATCH_LLM_URL":     "https://llm.example.test/v1/chat/completions",
		"JOB_MATCH_LLM_MODEL":   "test-model",
		"JOB_MATCH_LLM_API_KEY": "test-key",
		"JOB_MATCH_LLM_TIMEOUT": "2s",
	}
	getenv := func(k string) string { return env[k] }
	classifiers, err := classifiersFromEnv(getenv)
	if err != nil {
		t.Fatalf("classifiersFromEnv: %v", err)
	}
	jev, ok := classifiers["jev"].(*matching.LLMClassifier)
	if !ok {
		t.Fatalf("jev classifier type = %T", classifiers["jev"])
	}
	if jev == nil {
		t.Fatal("jev classifier is nil")
	}
	for _, timeout := range []string{"nope", "0s", "-1s"} {
		env["JOB_MATCH_LLM_TIMEOUT"] = timeout
		if got, err := classifiersFromEnv(getenv); err == nil || got != nil {
			t.Errorf("timeout %q: classifiers=%v err=%v", timeout, got, err)
		}
	}
	delete(env, "JOB_MATCH_LLM_TIMEOUT")
	if _, err := classifiersFromEnv(getenv); err != nil {
		t.Fatalf("default timeout: %v", err)
	}
}

func TestRunValidationAndDatabaseErrors(t *testing.T) {
	getenv := func(string) string { return "" }
	if err := run([]string{"-unknown"}, getenv, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown flag accepted")
	}
	if err := run(nil, getenv, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "database path required") {
		t.Fatalf("missing DB path err = %v", err)
	}
	goodEnv := map[string]string{"JOB_MATCH_LLM_URL": "https://llm.example.test", "JOB_MATCH_LLM_MODEL": "m", "JOB_MATCH_LLM_API_KEY": "k"}
	validEnv := func(k string) string { return goodEnv[k] }
	if err := run([]string{"-db", filepath.Join(t.TempDir(), "absent", "db.sqlite")}, validEnv, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "open database") {
		t.Fatalf("open DB err = %v", err)
	}
	badConfig := func(k string) string {
		if k == "JOB_DB_PATH" {
			return ":memory:"
		}
		return ""
	}
	if err := run(nil, badConfig, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "configure classifiers") {
		t.Fatalf("classifier config err = %v", err)
	}
}

func TestRunSuccessAndOutputError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.sqlite")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"JOB_DB_PATH": path, "JOB_MATCH_LLM_URL": "https://llm.example.test", "JOB_MATCH_LLM_MODEL": "m", "JOB_MATCH_LLM_API_KEY": "k"}
	getenv := func(k string) string { return env[k] }
	var output bytes.Buffer
	if err := run(nil, getenv, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := output.String(), "processed=0 matched=0 no_match=0 failures=0\n"; got != want {
		t.Fatalf("output=%q want %q", got, want)
	}
	if err := run(nil, getenv, failingWriter{}); err == nil {
		t.Fatal("expected output writer error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, os.ErrClosed }

var _ = time.Second
