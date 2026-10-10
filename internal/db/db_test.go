package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jobs.willwagner.me/internal/schema"
)

func TestEnsureCurrentFreshDatabase(t *testing.T) {
	conn := openTestDB(t)
	ctx := context.Background()
	if err := EnsureCurrent(ctx, conn); err != nil {
		t.Fatalf("EnsureCurrent: %v", err)
	}
	assertVersion(t, conn, schema.CurrentVersion)
	for _, table := range []string{"users", "user_configs", "user_history", "job_boards", "job_listings", "job_listing_versions", "job_matches", "job_history"} {
		assertTableExists(t, conn, table)
	}
}

func TestEnsureCurrentUpgradesPreviousSchema(t *testing.T) {
	conn := openTestDB(t)
	fixture, err := os.ReadFile(filepath.Join("testdata", "schema_v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(fixture)); err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	if err := EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatalf("EnsureCurrent: %v", err)
	}
	assertVersion(t, conn, schema.CurrentVersion)
	assertIndexExists(t, conn, "idx_job_history_service_started")
	var boards int
	if err := conn.QueryRow(`SELECT count(*) FROM job_boards WHERE board_url = 'https://example.com/jobs'`).Scan(&boards); err != nil {
		t.Fatal(err)
	}
	if boards != 1 {
		t.Fatalf("upgrade did not preserve fixture board, got %d", boards)
	}
}

func TestListingIdentityVersioningAndImmutability(t *testing.T) {
	conn := openCurrentTestDB(t)
	ctx := context.Background()
	res, err := conn.Exec(`INSERT INTO job_boards(board_url, company, ats_type, status) VALUES (?, ?, ?, ?)`, "https://boards.example", "Example", "lever", schema.BoardStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	boardID, _ := res.LastInsertId()
	found := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	first, err := UpsertListingVersion(ctx, conn, ListingVersionInput{
		CanonicalURL: "https://boards.example/jobs/123",
		BoardID:      boardID,
		Title:        "Engineer",
		Salary:       "$100k",
		Body:         "<p>Hello    world</p>",
		FoundAt:      found,
		MetadataJSON: `{"remote":true}`,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if !first.Inserted || first.Version != 1 {
		t.Fatalf("first version = %+v, want inserted version 1", first)
	}
	refresh, err := UpsertListingVersion(ctx, conn, ListingVersionInput{
		CanonicalURL: "https://boards.example/jobs/123",
		BoardID:      boardID,
		Title:        "Engineer",
		Salary:       "$100k",
		Body:         "Hello world",
		FoundAt:      found.Add(time.Hour),
		PostedAt:     sql.NullTime{Time: found.Add(-24 * time.Hour), Valid: true},
		MetadataJSON: `{"remote":false}`,
	})
	if err != nil {
		t.Fatalf("metadata refresh upsert: %v", err)
	}
	if refresh.Inserted || refresh.Version != 1 {
		t.Fatalf("refresh = %+v, want existing version 1", refresh)
	}
	changed, err := UpsertListingVersion(ctx, conn, ListingVersionInput{
		CanonicalURL: "https://boards.example/jobs/123",
		BoardID:      boardID,
		Title:        "Engineer",
		Salary:       "$120k",
		Body:         "Hello world",
		FoundAt:      found.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("changed upsert: %v", err)
	}
	if !changed.Inserted || changed.Version != 2 || changed.ListingID != first.ListingID {
		t.Fatalf("changed = %+v, want same listing new version 2", changed)
	}
	reverted, err := UpsertListingVersion(ctx, conn, ListingVersionInput{
		CanonicalURL: "https://boards.example/jobs/123",
		BoardID:      boardID,
		Title:        "Engineer",
		Salary:       "$100k",
		Body:         "<div>Hello world</div>",
		FoundAt:      found.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("reverted upsert: %v", err)
	}
	if !reverted.Inserted || reverted.Version != 3 || reverted.ListingID != first.ListingID {
		t.Fatalf("reverted = %+v, want same listing new version 3", reverted)
	}
	var versionRows int
	if err := conn.QueryRow(`SELECT count(*) FROM job_listing_versions WHERE job_listing_id = ?`, first.ListingID).Scan(&versionRows); err != nil {
		t.Fatal(err)
	}
	if versionRows != 3 {
		t.Fatalf("version rows = %d, want 3", versionRows)
	}
	wantBodies := map[int]string{1: "Hello world", 2: "Hello world", 3: "Hello world"}
	wantFoundAt := map[int]string{
		1: found.UTC().Format(time.RFC3339Nano),
		2: found.Add(2 * time.Hour).UTC().Format(time.RFC3339Nano),
		3: found.Add(3 * time.Hour).UTC().Format(time.RFC3339Nano),
	}
	wantSalary := map[int]string{1: "$100k", 2: "$120k", 3: "$100k"}
	for version := 1; version <= 3; version++ {
		var bodyText, foundAt, salary string
		if err := conn.QueryRow(`SELECT body_text, found_at, salary FROM job_listing_versions WHERE job_listing_id = ? AND version = ?`, first.ListingID, version).Scan(&bodyText, &foundAt, &salary); err != nil {
			t.Fatal(err)
		}
		if bodyText != wantBodies[version] || foundAt != wantFoundAt[version] || salary != wantSalary[version] {
			t.Fatalf("version %d = body %q found_at %q salary %q, want body %q found_at %q salary %q", version, bodyText, foundAt, salary, wantBodies[version], wantFoundAt[version], wantSalary[version])
		}
	}
}

func TestUniquenessAndChecks(t *testing.T) {
	conn := openCurrentTestDB(t)
	if _, err := conn.Exec(`INSERT INTO users(email, status) VALUES (?, ?)`, "person@example.com", schema.UserStatusPending); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO users(email, status) VALUES (?, ?)`, "person@example.com", schema.UserStatusPending); err == nil {
		t.Fatal("duplicate user email succeeded")
	}
	if _, err := conn.Exec(`INSERT INTO job_history(service, started_at, status, items_processed) VALUES (?, ?, ?, ?)`, "svc", time.Now().UTC().Format(time.RFC3339), "unknown", 1); err == nil {
		t.Fatal("invalid process status succeeded")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func openCurrentTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn := openTestDB(t)
	if err := EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

func assertVersion(t *testing.T, conn *sql.DB, want int) {
	t.Helper()
	got, err := Version(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
}

func assertTableExists(t *testing.T, conn *sql.DB, name string) {
	t.Helper()
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("table %s missing", name)
	}
}

func assertIndexExists(t *testing.T, conn *sql.DB, name string) {
	t.Helper()
	var count int
	if err := conn.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("index %s missing", name)
	}
}
