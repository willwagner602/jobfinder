package boards

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProviderRequestCoverage(t *testing.T) {
	if _, err := (AdzunaProvider{}).Discover(context.Background()); err == nil {
		t.Fatal("missing Adzuna credentials accepted")
	}
	if _, err := (SerpAPIProvider{}).Discover(context.Background()); err == nil {
		t.Fatal("missing SerpAPI key accepted")
	}
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"adzuna", `{"results":[{"id":"1","redirect_url":"https://boards.greenhouse.io/acme","company":{"display_name":"Acme"}},{"id":"2","redirect_url":"","company":{"display_name":"Skip"}}]}`, 1},
		{"serp", `{"jobs_results":[{"job_id":"1","company_name":"Acme","apply_options":[{"link":""},{"link":"https://jobs.lever.co/acme"}]},{"job_id":"2","company_name":"Missing link","apply_options":[]}]}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if tc.name == "adzuna" && r.URL.Query().Get("app_id") != "id" {
					t.Errorf("missing query credentials")
				}
				return jsonResponse(tc.body), nil
			})
			var got []Candidate
			var err error
			if tc.name == "adzuna" {
				got, err = (AdzunaProvider{Client: client, AppID: "id", AppKey: "key", Queries: []string{"x"}, MaxCalls: 1, WindowDays: 1}).Discover(context.Background())
			} else {
				got, err = (SerpAPIProvider{Client: client, APIKey: "key", Queries: []string{"x"}, PagesPerQuery: 1}).Discover(context.Background())
			}
			if err != nil || len(got) != tc.want {
				t.Fatalf("got %d, err %v", len(got), err)
			}
		})
	}
	bad := roundTripFunc(func(*http.Request) (*http.Response, error) { return jsonResponse("not json"), nil })
	if _, err := (AdzunaProvider{Client: bad, AppID: "id", AppKey: "key", Queries: []string{"x"}, Retry: RetryConfig{Attempts: 1}}).Discover(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestGetJSONFailureBranches(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client roundTripFunc
	}{
		{"transport", func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }},
		{"http exhaustion", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("slow down"))}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, sleeps := 0, 0
			client := roundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return tc.client(r) })
			err := getJSON(context.Background(), client, "https://example.test", new(any), RetryConfig{Attempts: 2, BaseDelay: time.Nanosecond, Sleep: func(time.Duration) { sleeps++ }})
			if err == nil || calls != 2 || sleeps != 1 {
				t.Fatalf("err=%v calls=%d sleeps=%d", err, calls, sleeps)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := getJSON(ctx, roundTripFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() }), "https://example.test", new(any), RetryConfig{Attempts: 1}); err == nil {
		t.Fatal("expected request creation/transport error")
	}
}

func TestFixtureProviderAndATSBranches(t *testing.T) {
	p := FixtureProvider{}
	if p.Name() != "fixture" {
		t.Fatal("default fixture name")
	}
	if _, err := p.Discover(context.Background()); err == nil {
		t.Fatal("expected missing fixture error")
	}
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (FixtureProvider{Path: path}).Discover(context.Background()); err == nil {
		t.Fatal("expected invalid fixture JSON")
	}
	if err := os.WriteFile(path, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FixtureProvider{Path: path}).Discover(ctx); err == nil {
		t.Fatal("expected canceled context")
	}
	for _, tc := range []struct {
		url, ats  string
		supported bool
	}{
		{"https://jobs.ashbyhq.com/acme", "ashby", true}, {"https://acme.myworkdayjobs.com/jobs", "workday", true}, {"https://acme.jobvite.com/jobs", "jobvite", true}, {"https://example.com/jobvite", "jobvite", true}, {"https://example.com/jobs", "unknown", false}, {":bad", "unknown", false},
	} {
		if got := detectATS(tc.url); got != tc.ats || supportedATS(got) != tc.supported {
			t.Errorf("%s: ATS=%s", tc.url, got)
		}
	}
}

func TestCandidateAndJSONErrorBranches(t *testing.T) {
	now := time.Now
	for _, c := range []Candidate{{URL: "https://x.test", Company: "X"}, {Provider: "p", URL: "https://x.test"}, {Provider: "p", Company: "X"}} {
		if _, err := c.Board(now); err == nil {
			t.Fatalf("accepted invalid candidate: %+v", c)
		}
	}
	b, err := (Candidate{Provider: " p ", Identifier: "id", URL: "https://EXAMPLE.test/a/?q=1#f", Supported: true, Metadata: map[string]string{" ": "ignored", "custom": "v"}}).Board(now)
	if err != nil {
		t.Fatal(err)
	}
	if b.Company != "id" || b.URL != "https://example.test/a" || b.Metadata["custom"] != "v" {
		t.Fatalf("board=%+v", b)
	}
	if err := WriteJSON(filepath.Join(t.TempDir(), "missing", "boards.json"), nil); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(t.TempDir(), "file", "boards.json"), nil); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(t.TempDir(), "as-file")
	if err := os.WriteFile(badDir, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(badDir, "boards.json"), nil); err == nil {
		t.Fatal("expected mkdir error")
	}
	if dir("file.json") != "." || dir("/file.json") != "/" {
		t.Fatal("dir edge cases")
	}
}
