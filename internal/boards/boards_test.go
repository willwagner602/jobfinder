package boards

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/schema"
)

type fakeProvider struct { name string; candidates []Candidate; err error }
func (p fakeProvider) Name() string { return p.name }
func (p fakeProvider) Discover(context.Context) ([]Candidate, error) { return p.candidates, p.err }

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ conn.Close() })
	if err := db.EnsureCurrent(context.Background(), conn); err != nil { t.Fatal(err) }
	return conn
}

func TestRunPersistsUnsupportedBoardsAndSynchronizesJSON(t *testing.T) {
	conn := testDB(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	jsonPath := filepath.Join(t.TempDir(), "job-boards.json")
	providers := []Provider{fakeProvider{name: "fake", candidates: []Candidate{
		{Provider: "fake", Identifier: "known", URL: "https://boards.greenhouse.io/acme?x=1#frag", Company: "Acme", ATSType: "greenhouse", Supported: true, Provenance: "fixture", DiscoveredAt: now},
		{Provider: "fake", Identifier: "unsupported", URL: "https://example.com/careers/", Company: "Example", ATSType: "custom", Supported: false, Provenance: "fixture", UpstreamStatus: "transient_failure", DiscoveredAt: now},
	}}}
	res, err := Run(context.Background(), conn, providers, Config{JSONPath: jsonPath, Now: func() time.Time { return now }})
	if err != nil { t.Fatalf("Run: %v", err) }
	if res.Inserted != 2 || res.JSONBoards != 2 { t.Fatalf("result = %+v, want 2 inserted and 2 JSON boards", res) }

	var unsupported int
	if err := conn.QueryRow(`SELECT count(*) FROM job_boards WHERE status = ? AND board_url = ?`, schema.BoardStatusUnsupported, "https://example.com/careers").Scan(&unsupported); err != nil { t.Fatal(err) }
	if unsupported != 1 { t.Fatalf("unsupported boards = %d, want 1", unsupported) }
	b, err := os.ReadFile(jsonPath)
	if err != nil { t.Fatal(err) }
	var boards []Board
	if err := json.Unmarshal(b, &boards); err != nil { t.Fatal(err) }
	if len(boards) != 2 { t.Fatalf("json boards = %d, want 2", len(boards)) }
	var historyStatus string
	if err := conn.QueryRow(`SELECT status FROM job_history WHERE service = ?`, ServiceName).Scan(&historyStatus); err != nil { t.Fatal(err) }
	if historyStatus != string(schema.ProcessStatusSuccess) { t.Fatalf("history status = %s", historyStatus) }
}

func TestCandidateValidation(t *testing.T) {
	_, err := (Candidate{Provider: "fake", URL: "not a url", Company: "Bad", Supported: false}).Board(time.Now)
	if err == nil { t.Fatal("expected invalid URL error") }
	_, err = (Candidate{Provider: "fake", URL: "https://example.com"}).Board(time.Now)
	if err == nil { t.Fatal("expected missing company or identifier error") }
}

func TestRunRecordsPartialWhenProviderFailsAfterReturningCandidates(t *testing.T) {
	conn := testDB(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	providers := []Provider{fakeProvider{name: "partial", candidates: []Candidate{{Provider: "partial", Identifier: "one", URL: "https://example.com/jobs", Company: "Example", Supported: false}}, err: os.ErrPermission}}
	_, err := Run(context.Background(), conn, providers, Config{JSONPath: filepath.Join(t.TempDir(), "boards.json"), Now: func() time.Time { return now }})
	if err == nil { t.Fatal("expected provider error") }
	var count int
	var status string
	if err := conn.QueryRow(`SELECT count(*) FROM job_boards`).Scan(&count); err != nil { t.Fatal(err) }
	if count != 1 { t.Fatalf("persisted boards = %d, want 1", count) }
	if err := conn.QueryRow(`SELECT status FROM job_history WHERE service = ?`, ServiceName).Scan(&status); err != nil { t.Fatal(err) }
	if status != string(schema.ProcessStatusPartial) { t.Fatalf("status = %s, want partial", status) }
}
