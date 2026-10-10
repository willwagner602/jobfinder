package boards

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)
func (f roundTripFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(body string) *http.Response { return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)} }

func TestSerpAPIProviderEnforcesQueryAndPageLimits(t *testing.T) {
	var calls int
	var starts []string
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		starts = append(starts, req.URL.Query().Get("start"))
		return jsonResponse(`{"jobs_results":[{"job_id":"1","company_name":"Acme","apply_options":[{"link":"https://jobs.lever.co/acme/1"}]}]}`), nil
	})
	provider := SerpAPIProvider{Client: client, APIKey: "key", Queries: []string{"q1","q2","q3","q4"}, PagesPerQuery: 9, Retry: RetryConfig{Attempts: 1}}
	candidates, err := provider.Discover(context.Background())
	if err != nil { t.Fatal(err) }
	if calls != DefaultSerpQueries*DefaultSerpPages { t.Fatalf("calls = %d, want %d", calls, DefaultSerpQueries*DefaultSerpPages) }
	if len(candidates) != calls { t.Fatalf("candidates = %d, want %d", len(candidates), calls) }
	for i, start := range starts {
		want := "0"; if i%2 == 1 { want = "10" }
		if start != want { t.Fatalf("start[%d] = %s, want %s", i, start, want) }
	}
}

func TestAdzunaProviderEnforcesMaxCallsAndWindow(t *testing.T) {
	var calls int
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if got := req.URL.Query().Get("max_days_old"); got != "8" { t.Fatalf("max_days_old = %s, want 8", got) }
		return jsonResponse(`{"results":[{"id":"1","redirect_url":"https://boards.greenhouse.io/acme/jobs/1","company":{"display_name":"Acme"}}]}`), nil
	})
	queries := make([]string, DefaultAdzunaMaxCalls+5)
	for i := range queries { queries[i] = "q" }
	provider := AdzunaProvider{Client: client, AppID: "id", AppKey: "key", Queries: queries, MaxCalls: DefaultAdzunaMaxCalls+5, WindowDays: 99, Retry: RetryConfig{Attempts: 1}}
	candidates, err := provider.Discover(context.Background())
	if err != nil { t.Fatal(err) }
	if calls != DefaultAdzunaMaxCalls { t.Fatalf("calls = %d, want %d", calls, DefaultAdzunaMaxCalls) }
	if len(candidates) != DefaultAdzunaMaxCalls { t.Fatalf("candidates = %d, want %d", len(candidates), DefaultAdzunaMaxCalls) }
}

func TestGetJSONRetriesWithBackoff(t *testing.T) {
	var calls int
	var slept []time.Duration
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 { return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("busy"))}, nil }
		return jsonResponse(`{"ok":true}`), nil
	})
	var out map[string]bool
	err := getJSON(context.Background(), client, "https://example.test", &out, RetryConfig{Attempts: 2, BaseDelay: time.Second, Sleep: func(d time.Duration){ slept = append(slept, d) }})
	if err != nil { t.Fatal(err) }
	if calls != 2 || len(slept) != 1 || slept[0] != time.Second { t.Fatalf("calls=%d slept=%v", calls, slept) }
}
