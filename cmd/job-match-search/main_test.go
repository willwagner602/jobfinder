package main

import (
	"testing"

	"jobs.willwagner.me/internal/matching"
)

func TestDefaultJevMatcherIsLLMClassifier(t *testing.T) {
	t.Setenv("JOB_MATCH_LLM_URL", "https://llm.example.test/v1/chat/completions")
	t.Setenv("JOB_MATCH_LLM_MODEL", "test-model")
	t.Setenv("JOB_MATCH_LLM_API_KEY", "test-key")
	classifiers, err := classifiersFromEnv()
	if err != nil {
		t.Fatalf("classifiersFromEnv: %v", err)
	}
	if _, ok := classifiers["jev"].(*matching.LLMClassifier); !ok {
		t.Fatalf("jev classifier type = %T, want *matching.LLMClassifier", classifiers["jev"])
	}
}

func TestDefaultJevMatcherRequiresLLMConfiguration(t *testing.T) {
	t.Setenv("JOB_MATCH_LLM_URL", "")
	t.Setenv("JOB_MATCH_LLM_MODEL", "test-model")
	t.Setenv("JOB_MATCH_LLM_API_KEY", "test-key")
	if classifiers, err := classifiersFromEnv(); err == nil || classifiers != nil {
		t.Fatalf("classifiersFromEnv = %v, %v; want configuration error", classifiers, err)
	}
}
