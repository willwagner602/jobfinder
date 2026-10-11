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
	for _, table := range []string{"users", "user_configs", "user_history", "job_boards", "job_listings", "job_listing_versions", "job_matches", "job_history", "site_config"} {
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
	if _, err := conn.Exec(`INSERT INTO job_boards(board_id, board_url, company, ats_type, status) VALUES (2, 'https://unsupported.example/jobs', 'Unsupported', 'custom', 'unsupported')`); err != nil {
		t.Fatalf("add unsupported fixture board: %v", err)
	}
	if err := EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatalf("EnsureCurrent: %v", err)
	}
	assertVersion(t, conn, schema.CurrentVersion)
	assertIndexExists(t, conn, "idx_job_history_service_started")
	assertNoForeignKeyViolations(t, conn)

	for boardURL, want := range map[string]struct {
		id     int64
		status schema.BoardStatus
	}{
		"https://example.com/jobs":         {id: 1, status: schema.BoardStatusActive},
		"https://unsupported.example/jobs": {id: 2, status: schema.BoardStatusUnsupported},
	} {
		var boardID int64
		var boardStatus string
		if err := conn.QueryRow(`SELECT board_id, status FROM job_boards WHERE board_url = ?`, boardURL).Scan(&boardID, &boardStatus); err != nil {
			t.Fatal(err)
		}
		if boardID != want.id || boardStatus != string(want.status) {
			t.Fatalf("upgrade did not preserve fixture board identity/status for %s: id=%d status=%q", boardURL, boardID, boardStatus)
		}
	}
	var versionFoundAt, versionBody, versionHash string
	if err := conn.QueryRow(`SELECT found_at, body_text, content_hash FROM job_listing_versions WHERE job_listing_id = 1 AND version = 1`).Scan(&versionFoundAt, &versionBody, &versionHash); err != nil {
		t.Fatal(err)
	}
	if versionFoundAt != "2025-01-02T03:04:05Z" || versionBody != "Build systems" || versionHash != "fixture-hash-a" {
		t.Fatalf("version row not preserved: found_at=%q body=%q hash=%q", versionFoundAt, versionBody, versionHash)
	}
	var matchRating int
	var matchDecision, matcherData string
	if err := conn.QueryRow(`SELECT rating, decision, job_match_data_json FROM job_matches WHERE user_id = 1 AND job_listing_id = 1 AND job_listing_version = 1`).Scan(&matchRating, &matchDecision, &matcherData); err != nil {
		t.Fatal(err)
	}
	if matchRating != 5 || matchDecision != "match" || matcherData != `{"reason":"fixture"}` {
		t.Fatalf("match row not preserved: rating=%d decision=%q data=%q", matchRating, matchDecision, matcherData)
	}
	if _, err := conn.Exec(`INSERT INTO job_listing_versions(job_listing_id, version, found_at, title, salary, body_text, content_hash) VALUES (1, 2, '2025-01-03T03:04:05Z', 'Engineer', '$100k', 'Build systems again', 'fixture-hash-a')`); err != nil {
		t.Fatalf("upgraded schema still rejects repeated historical content hash: %v", err)
	}
}

func TestSchemaV4SignupConfigAndTombstoneConstraints(t *testing.T) {
	conn := openCurrentTestDB(t)
	var disabled int
	if err := conn.QueryRow(`SELECT signups_disabled FROM site_config WHERE config_id = 1`).Scan(&disabled); err != nil {
		t.Fatal(err)
	}
	if disabled != 0 {
		t.Fatalf("fresh signups_disabled = %d, want 0", disabled)
	}
	if _, err := conn.Exec(`INSERT INTO site_config(config_id, signups_disabled) VALUES (2, 0)`); err == nil {
		t.Fatal("second site_config row succeeded")
	}
	if _, err := conn.Exec(`UPDATE site_config SET signups_disabled = 2 WHERE config_id = 1`); err == nil {
		t.Fatal("invalid signups_disabled value succeeded")
	}
	if _, err := conn.Exec(`INSERT INTO users(email, status) VALUES ('gone@example.com', ?)`, schema.UserStatusTombstoned); err != nil {
		t.Fatalf("tombstoned user status rejected: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO users(email, status) VALUES ('bad@example.com', 'deleted')`); err == nil {
		t.Fatal("invalid user status succeeded")
	}
}

func TestEnsureCurrentUpgradesSchemaV3ToV4PreservingUserRelatedRows(t *testing.T) {
	conn := openTestDB(t)
	fixture, err := os.ReadFile(filepath.Join("testdata", "schema_v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(fixture)); err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{2, 3} {
		if err := applyMigration(ctx, tx, version); err != nil {
			t.Fatalf("apply schema migration %d: %v", version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, conn, 3)
	if _, err := conn.Exec(`INSERT INTO user_configs(user_id, resume_text) VALUES (1, 'resume')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO user_history(user_id, event_type) VALUES (1, 'fixture')`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatalf("upgrade v3 to v4: %v", err)
	}
	assertVersion(t, conn, schema.CurrentVersion)
	assertNoForeignKeyViolations(t, conn)
	for table, query := range map[string]string{
		"users":        `SELECT count(*) FROM users WHERE user_id = 1 AND email = 'worker@example.com' AND status = 'live'`,
		"user_configs": `SELECT count(*) FROM user_configs WHERE user_id = 1 AND resume_text = 'resume'`,
		"user_history": `SELECT count(*) FROM user_history WHERE user_id = 1 AND event_type = 'fixture'`,
		"job_matches":  `SELECT count(*) FROM job_matches WHERE user_id = 1 AND job_listing_id = 1`,
	} {
		var rows int
		if err := conn.QueryRow(query).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 1 {
			t.Fatalf("%s rows preserved = %d, want 1", table, rows)
		}
	}
	if _, err := conn.Exec(`INSERT INTO users(email, status) VALUES ('gone-v3@example.com', ?)`, schema.UserStatusTombstoned); err != nil {
		t.Fatalf("upgraded v3 schema rejected tombstoned status: %v", err)
	}
	var disabled int
	if err := conn.QueryRow(`SELECT signups_disabled FROM site_config WHERE config_id = 1`).Scan(&disabled); err != nil {
		t.Fatal(err)
	}
	if disabled != 0 {
		t.Fatalf("upgraded signups_disabled = %d, want 0", disabled)
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

	res, err := conn.Exec(`INSERT INTO job_boards(board_url, company, ats_type) VALUES (?, ?, ?)`, "https://default.example/jobs", "Default", "greenhouse")
	if err != nil {
		t.Fatalf("insert default board status: %v", err)
	}
	defaultBoardID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	var defaultStatus string
	if err := conn.QueryRow(`SELECT status FROM job_boards WHERE board_id = ?`, defaultBoardID).Scan(&defaultStatus); err != nil {
		t.Fatal(err)
	}
	if defaultStatus != string(schema.BoardStatusDiscovered) {
		t.Fatalf("default board status = %q, want %q", defaultStatus, schema.BoardStatusDiscovered)
	}
	if _, err := conn.Exec(`INSERT INTO job_boards(board_url, company, ats_type, status) VALUES (?, ?, ?, ?)`, "https://discovered.example/jobs", "Discovered", "lever", schema.BoardStatusDiscovered); err != nil {
		t.Fatalf("discovered board status rejected: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO job_boards(board_url, company, ats_type, status) VALUES (?, ?, ?, ?)`, "https://invalid.example/jobs", "Invalid", "lever", "pending"); err == nil {
		t.Fatal("invalid board status succeeded")
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

func assertNoForeignKeyViolations(t *testing.T, conn *sql.DB) {
	t.Helper()
	rows, err := conn.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID int64
		var parent string
		var fkID int64
		if err := rows.Scan(&table, &rowID, &parent, &fkID); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("foreign key violation after migration: table=%s rowid=%d parent=%s fkid=%d", table, rowID, parent, fkID)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
