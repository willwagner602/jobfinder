package boards

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type HTTPDoer interface { Do(*http.Request) (*http.Response, error) }

type RetryConfig struct {
	Attempts int
	BaseDelay time.Duration
	Sleep func(time.Duration)
}

func (r RetryConfig) withDefaults() RetryConfig {
	if r.Attempts <= 0 { r.Attempts = 3 }
	if r.BaseDelay <= 0 { r.BaseDelay = 200 * time.Millisecond }
	if r.Sleep == nil { r.Sleep = time.Sleep }
	return r
}

type FixtureProvider struct { ProviderName string; Path string }
func (p FixtureProvider) Name() string { if p.ProviderName != "" { return p.ProviderName }; return "fixture" }
func (p FixtureProvider) Discover(ctx context.Context) ([]Candidate, error) {
	b, err := os.ReadFile(p.Path)
	if err != nil { return nil, err }
	var candidates []Candidate
	if err := json.Unmarshal(b, &candidates); err != nil { return nil, err }
	return candidates, ctx.Err()
}

type AdzunaProvider struct {
	Client HTTPDoer
	AppID string
	AppKey string
	Country string
	Queries []string
	MaxCalls int
	WindowDays int
	Retry RetryConfig
	Endpoint string
}
func (p AdzunaProvider) Name() string { return "adzuna" }
func (p AdzunaProvider) Discover(ctx context.Context) ([]Candidate, error) {
	if p.AppID == "" || p.AppKey == "" { return nil, fmt.Errorf("adzuna credentials are required") }
	client := p.Client; if client == nil { client = http.DefaultClient }
	country := p.Country; if country == "" { country = "us" }
	endpoint := p.Endpoint; if endpoint == "" { endpoint = "https://api.adzuna.com/v1/api/jobs/" + country + "/search/1" }
	maxCalls := p.MaxCalls; if maxCalls <= 0 || maxCalls > DefaultAdzunaMaxCalls { maxCalls = DefaultAdzunaMaxCalls }
	window := p.WindowDays; if window <= 0 || window > DefaultAdzunaWindowDays { window = DefaultAdzunaWindowDays }
	queries := p.Queries; if len(queries) == 0 { queries = []string{"software engineer", "golang", "remote software"} }
	var out []Candidate
	calls := 0
	for _, q := range queries {
		if calls >= maxCalls { break }
		u, _ := url.Parse(endpoint)
		params := u.Query(); params.Set("app_id", p.AppID); params.Set("app_key", p.AppKey); params.Set("what", q); params.Set("max_days_old", strconv.Itoa(window)); params.Set("results_per_page", "50"); u.RawQuery = params.Encode()
		var payload adzunaResponse
		if err := getJSON(ctx, client, u.String(), &payload, p.Retry); err != nil { return out, err }
		calls++
		for _, r := range payload.Results {
			candidate := candidateFromURL("adzuna", r.RedirectURL, r.Company.DisplayName, "adzuna:"+r.ID, q)
			if candidate.URL != "" { out = append(out, candidate) }
		}
	}
	return out, nil
}

type adzunaResponse struct { Results []struct { ID string `json:"id"`; RedirectURL string `json:"redirect_url"`; Company struct { DisplayName string `json:"display_name"` } `json:"company"` } `json:"results"` }

type SerpAPIProvider struct {
	Client HTTPDoer
	APIKey string
	Queries []string
	PagesPerQuery int
	Retry RetryConfig
	Endpoint string
}
func (p SerpAPIProvider) Name() string { return "serpapi" }
func (p SerpAPIProvider) Discover(ctx context.Context) ([]Candidate, error) {
	if p.APIKey == "" { return nil, fmt.Errorf("serpapi api key is required") }
	client := p.Client; if client == nil { client = http.DefaultClient }
	endpoint := p.Endpoint; if endpoint == "" { endpoint = "https://serpapi.com/search.json" }
	queries := p.Queries; if len(queries) == 0 { queries = []string{"software engineer remote", "golang jobs", "climate software jobs"} }
	if len(queries) > DefaultSerpQueries { queries = queries[:DefaultSerpQueries] }
	pages := p.PagesPerQuery; if pages <= 0 || pages > DefaultSerpPages { pages = DefaultSerpPages }
	var out []Candidate
	for _, q := range queries {
		for page := 0; page < pages; page++ {
			u, _ := url.Parse(endpoint)
			params := u.Query(); params.Set("engine", "google_jobs"); params.Set("q", q); params.Set("api_key", p.APIKey); params.Set("start", strconv.Itoa(page*10)); u.RawQuery = params.Encode()
			var payload serpResponse
			if err := getJSON(ctx, client, u.String(), &payload, p.Retry); err != nil { return out, err }
			for _, job := range payload.JobsResults {
				link := firstApplyLink(job.ApplyOptions)
				candidate := candidateFromURL("serpapi", link, job.CompanyName, "serpapi:"+job.JobID, q)
				if candidate.URL != "" { out = append(out, candidate) }
			}
		}
	}
	return out, nil
}

type serpResponse struct { JobsResults []struct { JobID string `json:"job_id"`; CompanyName string `json:"company_name"`; ApplyOptions []struct { Link string `json:"link"` } `json:"apply_options"` } `json:"jobs_results"` }

func firstApplyLink(options []struct{ Link string `json:"link"` }) string { for _, o := range options { if o.Link != "" { return o.Link } }; return "" }

func candidateFromURL(provider, rawURL, company, identifier, provenance string) Candidate {
	if rawURL == "" || company == "" { return Candidate{} }
	ats := detectATS(rawURL)
	return Candidate{Provider: provider, Identifier: identifier, URL: rawURL, Company: company, ATSType: ats, Supported: supportedATS(ats), Provenance: provenance, UpstreamStatus: "ok"}
}

func detectATS(raw string) string {
	u, err := url.Parse(raw); if err != nil { return "unknown" }
	host := strings.ToLower(u.Host)
	path := strings.ToLower(u.Path)
	switch {
	case strings.Contains(host, "greenhouse.io") || strings.Contains(host, "greenhouse"):
		return "greenhouse"
	case strings.Contains(host, "lever.co"):
		return "lever"
	case strings.Contains(host, "ashbyhq.com"):
		return "ashby"
	case strings.Contains(host, "myworkdayjobs.com"):
		return "workday"
	case strings.Contains(host, "jobvite.com") || strings.Contains(path, "jobvite"):
		return "jobvite"
	default:
		return "unknown"
	}
}

func supportedATS(ats string) bool { switch ats { case "greenhouse", "lever", "ashby", "workday", "jobvite": return true; default: return false } }

func getJSON(ctx context.Context, client HTTPDoer, target string, out any, retry RetryConfig) error {
	retry = retry.withDefaults()
	var last error
	for attempt := 0; attempt < retry.Attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil { return err }
		resp, err := client.Do(req)
		if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(out)
		}
		if resp != nil { body, _ := io.ReadAll(io.LimitReader(resp.Body, 512)); resp.Body.Close(); last = fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body))) } else { last = err }
		if attempt+1 < retry.Attempts { retry.Sleep(retry.BaseDelay << attempt) }
	}
	return last
}
