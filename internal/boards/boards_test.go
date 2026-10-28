package boards

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jobfinder/internal/db"
	"jobfinder/internal/schema"
)

type fakeProvider struct {
	name       string
	candidates []Candidate
	err        error
}

func (p fakeProvider) Name() string                                  { return p.name }
func (p fakeProvider) Discover(context.Context) ([]Candidate, error) { return p.candidates, p.err }

func testDB(t *testing.T) *sql.DB {
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

func TestRunPersistsDiscoveredAndUnsupportedBoardsAndSynchronizesJSON(t *testing.T) {
	conn := testDB(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	jsonPath := filepath.Join(t.TempDir(), "job-boards.json")
	providers := []Provider{fakeProvider{name: "fake", candidates: []Candidate{
		{Provider: "fake", Identifier: "known", URL: "https://boards.greenhouse.io/acme?x=1#frag", Company: "Acme", ATSType: "greenhouse", Supported: true, Provenance: "fixture", DiscoveredAt: now},
		{Provider: "fake", Identifier: "unsupported", URL: "https://example.com/careers/", Company: "Example", ATSType: "custom", Supported: false, Provenance: "fixture", UpstreamStatus: "transient_failure", DiscoveredAt: now},
	}}}
	res, err := Run(context.Background(), conn, providers, Config{JSONPath: jsonPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Inserted != 2 || res.JSONBoards != 2 {
		t.Fatalf("result = %+v, want 2 inserted and 2 JSON boards", res)
	}

	var discovered int
	if err := conn.QueryRow(`SELECT count(*) FROM job_boards WHERE status = ? AND board_url = ?`, schema.BoardStatusDiscovered, "https://boards.greenhouse.io/acme").Scan(&discovered); err != nil {
		t.Fatal(err)
	}
	if discovered != 1 {
		t.Fatalf("discovered boards = %d, want 1", discovered)
	}
	var unsupported int
	if err := conn.QueryRow(`SELECT count(*) FROM job_boards WHERE status = ? AND board_url = ?`, schema.BoardStatusUnsupported, "https://example.com/careers").Scan(&unsupported); err != nil {
		t.Fatal(err)
	}
	if unsupported != 1 {
		t.Fatalf("unsupported boards = %d, want 1", unsupported)
	}
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var boards []Board
	if err := json.Unmarshal(b, &boards); err != nil {
		t.Fatal(err)
	}
	if len(boards) != 2 {
		t.Fatalf("json boards = %d, want 2", len(boards))
	}
	var historyStatus string
	if err := conn.QueryRow(`SELECT status FROM job_history WHERE service = ?`, ServiceName).Scan(&historyStatus); err != nil {
		t.Fatal(err)
	}
	if historyStatus != string(schema.ProcessStatusSuccess) {
		t.Fatalf("history status = %s", historyStatus)
	}
}

func TestRunUpdatesAndDeduplicatesCandidates(t *testing.T) {
	conn := testDB(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	original := Candidate{Provider: "p", Identifier: "x", URL: "https://example.com/jobs", Company: "Old", Supported: false, DiscoveredAt: now}
	if _, err := Run(context.Background(), conn, []Provider{fakeProvider{name: "p", candidates: []Candidate{original}}}, Config{JSONPath: filepath.Join(t.TempDir(), "boards.json"), Now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	original.Company = "New"
	res, err := Run(context.Background(), conn, []Provider{fakeProvider{name: "p", candidates: []Candidate{original, original}}}, Config{JSONPath: filepath.Join(t.TempDir(), "boards.json"), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || res.Candidates != 2 {
		t.Fatalf("result=%+v", res)
	}
	var company string
	if err := conn.QueryRow(`SELECT company FROM job_boards`).Scan(&company); err != nil || company != "New" {
		t.Fatalf("company=%q err=%v", company, err)
	}
}

func TestRunValidationAndPersistenceErrors(t *testing.T) {
	if _, err := Run(context.Background(), nil, []Provider{fakeProvider{name: "p"}}, Config{}); err == nil {
		t.Fatal("expected nil db error")
	}
	conn := testDB(t)
	if _, err := Run(context.Background(), conn, nil, Config{}); err == nil {
		t.Fatal("expected no provider error")
	}
	if _, err := Run(context.Background(), conn, []Provider{fakeProvider{name: "p", candidates: []Candidate{{Provider: "", URL: "https://example.com", Company: "X"}}}}, Config{JSONPath: filepath.Join(t.TempDir(), "x.json")}); err == nil {
		t.Fatal("expected invalid candidate error")
	}
}

func TestRunPreservesAlreadyActiveBoards(t *testing.T) {
	conn := testDB(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := conn.Exec(`INSERT INTO job_boards (board_url, company, ats_type, status, metadata_json, discovered_at, updated_at) VALUES (?, ?, ?, ?, '{}', ?, ?)`, "https://jobs.lever.co/acme/1", "Acme", "lever", schema.BoardStatusActive, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	providers := []Provider{fakeProvider{name: "fake", candidates: []Candidate{{Provider: "fake", Identifier: "known", URL: "https://jobs.lever.co/acme/1", Company: "Acme", ATSType: "lever", Supported: true, DiscoveredAt: now}}}}
	_, err := Run(context.Background(), conn, providers, Config{JSONPath: filepath.Join(t.TempDir(), "boards.json"), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var status string
	if err := conn.QueryRow(`SELECT status FROM job_boards WHERE board_url = ?`, "https://jobs.lever.co/acme/1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(schema.BoardStatusActive) {
		t.Fatalf("status = %s, want active", status)
	}
}

func TestCandidateValidation(t *testing.T) {
	_, err := (Candidate{Provider: "fake", URL: "not a url", Company: "Bad", Supported: false}).Board(time.Now)
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
	_, err = (Candidate{Provider: "fake", URL: "https://example.com"}).Board(time.Now)
	if err == nil {
		t.Fatal("expected missing company or identifier error")
	}
}

func TestRunRecordsPartialWhenProviderFailsAfterReturningCandidates(t *testing.T) {
	conn := testDB(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	providers := []Provider{fakeProvider{name: "partial", candidates: []Candidate{{Provider: "partial", Identifier: "one", URL: "https://example.com/jobs", Company: "Example", Supported: false}}, err: os.ErrPermission}}
	_, err := Run(context.Background(), conn, providers, Config{JSONPath: filepath.Join(t.TempDir(), "boards.json"), Now: func() time.Time { return now }})
	if err == nil {
		t.Fatal("expected provider error")
	}
	var count int
	var status string
	if err := conn.QueryRow(`SELECT count(*) FROM job_boards`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("persisted boards = %d, want 1", count)
	}
	if err := conn.QueryRow(`SELECT status FROM job_history WHERE service = ?`, ServiceName).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(schema.ProcessStatusPartial) {
		t.Fatalf("status = %s, want partial", status)
	}
}
