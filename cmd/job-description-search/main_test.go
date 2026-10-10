package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/jobs"
)

func TestMainInvokesRun(t *testing.T) {
	old := flag.CommandLine
	defer func() { flag.CommandLine = old }()
	flag.CommandLine = flag.NewFlagSet("job-description-search-test", flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"job-description-search", "-db", filepath.Join(t.TempDir(), "main.sqlite"), "-since", "2025-01-01"}
	main()
}

func TestRunValidatesInputAndRuns(t *testing.T) {
	if err := run("unused.sqlite", "not-a-date", ""); err == nil {
		t.Fatal("run should reject an invalid date")
	}
	if err := run(t.TempDir(), "2025-01-01", ""); err == nil {
		t.Fatal("run should reject a directory as a database file")
	}
	path := t.TempDir() + "/jobs.sqlite"
	if err := run(path, "2025-01-01", ""); err != nil {
		t.Fatalf("run with empty database: %v", err)
	}
}

func TestRunRecordsJobErrors(t *testing.T) {
	path := t.TempDir() + "/jobs.sqlite"
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO job_boards(board_url, company, ats_type, status) VALUES ('https://example.test', 'Unknown', 'bogus', 'active')`); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := run(path, "2025-01-01", ""); err == nil {
		t.Fatal("run should return provider errors")
	}
}

func TestStartHealth(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	server := startHealth(addr)
	defer server.Shutdown(context.Background())
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("status=%d", resp.StatusCode)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("health endpoint did not start")
}

func TestStartHealthLogsListenFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	defer listener.Close()
	server := startHealth(addr)
	defer server.Close()
	time.Sleep(20 * time.Millisecond)
}

func TestRecordHistoryStatusesAndJSON(t *testing.T) {
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	cases := []struct {
		name   string
		result jobs.Result
		runErr error
		status string
	}{
		{"success", jobs.Result{Seen: 2, Inserted: 1, Updated: 1}, nil, "success"},
		{"failure", jobs.Result{Seen: 1, Inserted: 1}, context.DeadlineExceeded, "failure"},
		{"partial", jobs.Result{Seen: 3, Inserted: 1}, context.DeadlineExceeded, "partial"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := recordHistory(context.Background(), conn, started, tc.result, tc.runErr); err != nil {
				t.Fatal(err)
			}
			var status string
			var count int
			var raw, errRaw sql.NullString
			if err := conn.QueryRow(`SELECT status, items_processed, data_json, error_json FROM job_history ORDER BY rowid DESC LIMIT 1`).Scan(&status, &count, &raw, &errRaw); err != nil {
				t.Fatal(err)
			}
			if status != tc.status || count != tc.result.Seen {
				t.Fatalf("history status=%q count=%d", status, count)
			}
			var payload map[string]int
			if err := json.Unmarshal([]byte(raw.String), &payload); err != nil || payload["seen"] != tc.result.Seen {
				t.Fatalf("data=%q err=%v", raw.String, err)
			}
			if (tc.runErr != nil) != errRaw.Valid {
				t.Fatalf("error_json valid=%v, want %v", errRaw.Valid, tc.runErr != nil)
			}
		})
	}
}
