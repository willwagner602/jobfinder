package matching

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"jobs.willwagner.me/internal/schema"
)

const defaultLLMTimeout = 30 * time.Second

// LLMConfig configures an OpenAI-compatible chat-completions classifier.
// Endpoint is the full HTTP URL for the chat completions API. APIKey is read
// from the process environment by the executable and is only sent as an
// Authorization header; it is never stored in match result data.
type LLMConfig struct {
	Endpoint string
	Model    string
	APIKey   string
	Timeout  time.Duration
	Client   *http.Client
}

type LLMClassifier struct {
	config LLMConfig
	client *http.Client
}

func NewLLMClassifier(config LLMConfig) (*LLMClassifier, error) {
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, errors.New("LLM endpoint is required")
	}
	u, err := url.Parse(config.Endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("LLM endpoint must be an absolute http(s) URL")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("LLM model is required")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("LLM API key is required")
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultLLMTimeout
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	return &LLMClassifier{config: config, client: client}, nil
}

func NewLLMClassifierFromEnv() (*LLMClassifier, error) {
	timeout := defaultLLMTimeout
	if raw := strings.TrimSpace(os.Getenv("JOB_MATCH_LLM_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("parse JOB_MATCH_LLM_TIMEOUT: %w", err)
		}
		timeout = parsed
	}
	return NewLLMClassifier(LLMConfig{
		Endpoint: os.Getenv("JOB_MATCH_LLM_URL"),
		Model:    os.Getenv("JOB_MATCH_LLM_MODEL"),
		APIKey:   os.Getenv("JOB_MATCH_LLM_API_KEY"),
		Timeout:  timeout,
	})
}

func (c *LLMClassifier) Classify(ctx context.Context, req Request) (Result, error) {
	body, err := json.Marshal(chatCompletionRequest{
		Model: c.config.Model,
		Messages: []chatMessage{
			{Role: "system", Content: "You classify whether a job is a fit for a candidate. Return only JSON with decision match or no_match, rating 1-5 for match and 0 for no_match, and reason."},
			{Role: "user", Content: classifierPrompt(req)},
		},
		Temperature: 0,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
	})
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 1<<20)
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("LLM provider returned HTTP %d: %s", resp.StatusCode, truncateForError(string(respBody)))
	}
	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return Result{}, fmt.Errorf("decode LLM provider response: %w", err)
	}
	if len(completion.Choices) == 0 || strings.TrimSpace(completion.Choices[0].Message.Content) == "" {
		return Result{}, errors.New("LLM provider response did not include message content")
	}
	var parsed llmDecision
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &parsed); err != nil {
		return Result{}, fmt.Errorf("decode LLM classification JSON: %w", err)
	}
	return parsed.result(c.config.Model)
}

func classifierPrompt(req Request) string {
	payload := map[string]any{
		"candidate": map[string]any{
			"resume_text":     req.ResumeText,
			"experience_text": req.ExperienceText,
			"desired_titles":  req.DesiredTitles,
		},
		"job": map[string]any{
			"title":     req.Title,
			"company":   req.Company,
			"body_text": req.BodyText,
		},
		"instructions": "Classify this job for the candidate. Use decision match only when the job is plausibly compatible; otherwise use no_match.",
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type llmDecision struct {
	Decision string          `json:"decision"`
	Rating   int             `json:"rating"`
	Reason   string          `json:"reason"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

func (d llmDecision) result(model string) (Result, error) {
	decision := schema.JobMatchDecision(d.Decision)
	switch decision {
	case schema.JobMatchDecisionMatch:
		if d.Rating < 1 || d.Rating > 5 {
			return Result{}, fmt.Errorf("LLM match rating %d outside 1-5", d.Rating)
		}
	case schema.JobMatchDecisionNoMatch:
		d.Rating = 0
	default:
		return Result{}, fmt.Errorf("LLM decision %q is not supported", d.Decision)
	}
	return Result{
		Decision: decision,
		Rating:   d.Rating,
		Data: mustJSON(map[string]any{
			"decision": d.Decision,
			"rating":   d.Rating,
			"reason":   d.Reason,
		}),
		Metadata: mustJSON(map[string]string{
			"provider_contract": "openai_chat_completions",
			"model":             model,
		}),
	}, nil
}

func truncateForError(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 200 {
		return s
	}
	return s[:200] + "..."
}
