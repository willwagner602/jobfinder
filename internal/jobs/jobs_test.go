package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"jobs.willwagner.me/internal/db"
)

func TestProvidersParseRequiredSources(t *testing.T) {
	var climateQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/greenhouse/jobs":
			fmt.Fprint(w, `{"jobs":[{"id":11,"title":"GH Engineer","absolute_url":"https://example.com/gh?utm_source=x","content":"<p>Full GH JD</p>","first_published":"2025-01-02T00:00:00Z"}]}`)
		case "/lever/postings":
			fmt.Fprint(w, `[{"id":"lev1","text":"Lever Engineer","hostedUrl":"https://example.com/lever","description":"<p>Full Lever JD</p>","createdAt":1735776000000}]`)
		case "/ashby/jobs":
			fmt.Fprint(w, `{"jobs":[{"id":"ash1","title":"Ashby Engineer","jobUrl":"https://example.com/ashby","descriptionHtml":"<p>Full Ashby JD</p>","updatedAt":"2025-01-03T00:00:00Z"}]}`)
		case "/workday/jobs":
			fmt.Fprint(w, `{"jobPostings":[{"title":"Workday Engineer","externalPath":"/wd/1","postedOn":"2025-01-04","summary":"Summary only"}]}`)
		case "/jobvite/jobs.json":
			fmt.Fprint(w, `{"jobs":[{"id":"jv1","title":"Jobvite Engineer","url":"https://example.com/jobvite","description":"Jobvite listing","date":"2025-01-05"}]}`)
		case "/climate/api/jobs":
			climateQuery = r.URL.RawQuery
			fmt.Fprint(w, `{"jobs":[{"id":"cb1","title":"Climate Engineer","url":"https://example.com/climate","description":"Climate listing","posted_at":"2025-01-06"}]}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.String())
		}
	}))
	defer server.Close()

	cases := []struct {
		name     string
		provider Provider
		url      string
		wantBody string
	}{
		{"greenhouse", GreenhouseProvider{}, server.URL + "/greenhouse", "Full GH JD"},
		{"lever", LeverProvider{}, server.URL + "/lever", "Full Lever JD"},
		{"ashby", AshbyProvider{}, server.URL + "/ashby", "Full Ashby JD"},
		{"workday", WorkdayProvider{}, server.URL + "/workday", "Summary only"},
		{"jobvite", JobviteProvider{}, server.URL + "/jobvite", "Jobvite listing"},
		{"climatebase", ClimatebaseProvider{}, server.URL + "/climate", "Climate listing"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.provider.Fetch(context.Background(), server.Client(), Board{ID: int64(i + 1), URL: tc.url}, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatalf("listings = %d, want 1", len(got))
			}
			if !strings.Contains(got[0].BodyHTML, tc.wantBody) || got[0].CanonicalURL == "" || got[0].Title == "" {
				t.Fatalf("listing not populated: %+v", got[0])
			}
		})
	}
	if climateQuery != "posted_after=2025-01-01" {
		t.Fatalf("climatebase query = %q, want server-side posted_after", climateQuery)
	}
}

func TestHackerNewsCacheAndRetry(t *testing.T) {
	var calls int32
	page := `<html><body>
<tr class="athing comtr" id="100"><td><span class="commtext c00"><a href="https://acme.example/jobs">Acme</a> is hiring remote Go engineers</span></td></tr>
<tr class="athing comtr" id="101"><td><span class="commtext c00">Just a discussion comment</span></td></tr>
</body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, page)
	}))
	defer server.Close()

	provider := HackerNewsProvider{Cache: NewCommentCache()}
	first, err := provider.Fetch(context.Background(), server.Client(), Board{URL: server.URL}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].CanonicalURL != "https://acme.example/jobs" {
		t.Fatalf("first = %+v", first)
	}
	second, err := provider.Fetch(context.Background(), server.Client(), Board{URL: server.URL}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || len(provider.Cache.positive) != 1 || len(provider.Cache.negative) != 1 {
		t.Fatalf("cache state positive=%d negative=%d second=%+v", len(provider.Cache.positive), len(provider.Cache.negative), second)
	}
	if calls < 2 {
		t.Fatalf("retry did not occur, calls=%d", calls)
	}
}

func TestIngestBoardUsesCanonicalURLAndVersionContract(t *testing.T) {
	conn := openCurrentTestDB(t)
	res, err := conn.Exec(`INSERT INTO job_boards(board_url, company, ats_type, status) VALUES (?, ?, ?, ?)`, "https://board.example", "Example", "greenhouse", "active")
	if err != nil {
		t.Fatal(err)
	}
	boardID, _ := res.LastInsertId()
	provider := staticProvider{listings: []Listing{{CanonicalURL: "https://example.com/job?utm_campaign=x", Title: "Engineer", Salary: "$100k", BodyHTML: "<p>Hello world</p>", Metadata: map[string]string{"provider": "test"}}}}
	result, err := IngestBoard(context.Background(), conn, nil, Board{ID: boardID}, time.Time{}, provider)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted != 1 || result.Updated != 0 {
		t.Fatalf("first result = %+v", result)
	}
	provider.listings[0].BodyHTML = "Hello    world"
	result, err = IngestBoard(context.Background(), conn, nil, Board{ID: boardID}, time.Time{}, provider)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted != 0 || result.Updated != 1 {
		t.Fatalf("markup refresh result = %+v", result)
	}
	provider.listings[0].Salary = "$120k"
	result, err = IngestBoard(context.Background(), conn, nil, Board{ID: boardID}, time.Time{}, provider)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted != 1 || result.Updated != 0 {
		t.Fatalf("changed result = %+v", result)
	}
	var versions int
	if err := conn.QueryRow(`SELECT count(*) FROM job_listing_versions`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 2 {
		t.Fatalf("versions = %d, want 2", versions)
	}
}

type staticProvider struct{ listings []Listing }

func (p staticProvider) Fetch(context.Context, *http.Client, Board, time.Time) ([]Listing, error) { return p.listings, nil }

func openCurrentTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	return conn
}
