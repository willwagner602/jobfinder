package matching

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jobs.willwagner.me/internal/schema"
)

func TestLLMClassifierUsesProviderNeutralHTTPContract(t *testing.T) {
	var gotAuth, gotModel string
	var gotMessages []chatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotModel = req.Model
		gotMessages = req.Messages
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"decision\":\"match\",\"rating\":5,\"reason\":\"strong fit\"}"}}]}`))
	}))
	defer server.Close()
	classifier, err := NewLLMClassifier(LLMConfig{Endpoint: server.URL, Model: "test-model", APIKey: "secret", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewLLMClassifier: %v", err)
	}

	result, err := classifier.Classify(context.Background(), Request{ResumeText: "Go", ExperienceText: "SQLite", DesiredTitles: []string{"engineer"}, Title: "Backend Engineer", Company: "Example", BodyText: "Build Go services"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotModel != "test-model" {
		t.Fatalf("model = %q", gotModel)
	}
	if len(gotMessages) != 2 || gotMessages[0].Role != "system" || gotMessages[1].Role != "user" {
		t.Fatalf("messages = %+v", gotMessages)
	}
	if !strings.Contains(gotMessages[1].Content, "Backend Engineer") || !strings.Contains(gotMessages[1].Content, "desired_titles") {
		t.Fatalf("prompt missing request data: %s", gotMessages[1].Content)
	}
	if result.Decision != schema.JobMatchDecisionMatch || result.Rating != 5 {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(string(result.Metadata), "secret") || !strings.Contains(string(result.Metadata), "openai_chat_completions") {
		t.Fatalf("metadata not safe/provider-neutral: %s", result.Metadata)
	}
}

func TestLLMClassifierNoMatchAndProviderErrors(t *testing.T) {
	t.Run("no match parses rating as null persistence input", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"decision\":\"no_match\",\"rating\":0,\"reason\":\"not aligned\"}"}}]}`))
		}))
		defer server.Close()
		classifier, err := NewLLMClassifier(LLMConfig{Endpoint: server.URL, Model: "model", APIKey: "key"})
		if err != nil {
			t.Fatal(err)
		}
		result, err := classifier.Classify(context.Background(), Request{Title: "Sales"})
		if err != nil {
			t.Fatalf("Classify: %v", err)
		}
		if result.Decision != schema.JobMatchDecisionNoMatch || result.Rating != 0 {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("http provider failure returns error for retry", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "temporary outage", http.StatusTooManyRequests)
		}))
		defer server.Close()
		classifier, err := NewLLMClassifier(LLMConfig{Endpoint: server.URL, Model: "model", APIKey: "key"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = classifier.Classify(context.Background(), Request{Title: "Engineer"})
		if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("malformed classifier JSON returns error for retry", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not-json"}}]}`))
		}))
		defer server.Close()
		classifier, err := NewLLMClassifier(LLMConfig{Endpoint: server.URL, Model: "model", APIKey: "key"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = classifier.Classify(context.Background(), Request{Title: "Engineer"})
		if err == nil || !strings.Contains(err.Error(), "decode LLM classification JSON") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestLLMClassifierConfigValidation(t *testing.T) {
	cases := []LLMConfig{
		{Endpoint: "", Model: "m", APIKey: "k"},
		{Endpoint: "not-a-url", Model: "m", APIKey: "k"},
		{Endpoint: "https://example.test", Model: "", APIKey: "k"},
		{Endpoint: "https://example.test", Model: "m", APIKey: ""},
	}
	for _, tc := range cases {
		if classifier, err := NewLLMClassifier(tc); err == nil || classifier != nil {
			t.Fatalf("NewLLMClassifier(%+v) = %v, %v; want error", tc, classifier, err)
		}
	}
}
