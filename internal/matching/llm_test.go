package matching

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLLMClassifierResponseErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"http", strings.Repeat("x", 250), 500},
		{"invalid provider json", "{", 200},
		{"missing choices", `{"choices":[]}`, 200},
		{"missing content", `{"choices":[{"message":{"content":" "}}]}`, 200},
		{"invalid decision json", `{"choices":[{"message":{"content":"{"}}]}`, 200},
		{"invalid decision", `{"choices":[{"message":{"content":"{\"decision\":\"wat\"}"}}]}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c, err := NewLLMClassifier(LLMConfig{Endpoint: srv.URL, Model: "m", APIKey: "k"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Classify(context.Background(), Request{}); err == nil {
				t.Fatal("expected classification error")
			}
		})
	}
}

func TestLLMClassifierTransportAndConfig(t *testing.T) {
	if _, err := NewLLMClassifier(LLMConfig{Endpoint: "http://[::1", Model: "m", APIKey: "k"}); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if _, err := NewLLMClassifier(LLMConfig{Endpoint: "ftp://example.test", Model: "m", APIKey: "k"}); err == nil {
		t.Fatal("non-http URL accepted")
	}
	if _, err := NewLLMClassifier(LLMConfig{Endpoint: "http://example.test", Model: "m", APIKey: "k", Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	c, err := NewLLMClassifier(LLMConfig{Endpoint: srv.URL, Model: "m", APIKey: "k", Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if _, err := c.Classify(context.Background(), Request{}); err == nil {
		t.Fatal("closed server accepted")
	}
}

func TestNewLLMClassifierFromEnvErrors(t *testing.T) {
	t.Setenv("JOB_MATCH_LLM_TIMEOUT", "bad")
	if _, err := NewLLMClassifierFromEnv(); err == nil {
		t.Fatal("invalid timeout accepted")
	}
	for _, k := range []string{"JOB_MATCH_LLM_TIMEOUT", "JOB_MATCH_LLM_URL", "JOB_MATCH_LLM_MODEL", "JOB_MATCH_LLM_API_KEY"} {
		t.Setenv(k, "")
	}
	if _, err := NewLLMClassifierFromEnv(); err == nil {
		t.Fatal("missing config accepted")
	}
	_ = os.Getenv("JOB_MATCH_LLM_URL")
}
