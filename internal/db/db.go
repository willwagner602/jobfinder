package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"jobs.willwagner.me/internal/schema"
)

const DriverName = "sqlite3"

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open(DriverName, path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func EnsureCurrent(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	version, err := currentVersion(ctx, conn)
	if err != nil {
		return err
	}
	if version > schema.CurrentVersion {
		return fmt.Errorf("database schema version %d is newer than application schema version %d", version, schema.CurrentVersion)
	}
	if version == schema.CurrentVersion {
		return foreignKeyCheck(ctx, conn)
	}

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for version < schema.CurrentVersion {
		next := version + 1
		if err := applyMigration(ctx, tx, next); err != nil {
			return fmt.Errorf("apply schema migration %d: %w", next, err)
		}
		version = next
	}
	if err := foreignKeyCheck(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
	return err
}

func Version(ctx context.Context, conn *sql.DB) (int, error) {
	return currentVersion(ctx, conn)
}

type foreignKeyChecker interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func foreignKeyCheck(ctx context.Context, q foreignKeyChecker) error {
	rows, err := q.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID int64
		var parent string
		var fkID int64
		if err := rows.Scan(&table, &rowID, &parent, &fkID); err != nil {
			return err
		}
		return fmt.Errorf("foreign key check failed: table=%s rowid=%d parent=%s fkid=%d", table, rowID, parent, fkID)
	}
	return rows.Err()
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func currentVersion(ctx context.Context, q queryer) (int, error) {
	var exists int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, nil
	}
	var version int
	if err := q.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func applyMigration(ctx context.Context, tx *sql.Tx, version int) error {
	switch version {
	case 1:
		return execAll(ctx, tx, migration1)
	case 2:
		return execAll(ctx, tx, migration2)
	case 3:
		return execAll(ctx, tx, migration3)
	case 4:
		return execAll(ctx, tx, migration4)
	default:
		return fmt.Errorf("no migration for version %d", version)
	}
}

func execAll(ctx context.Context, tx *sql.Tx, statements []string) error {
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

var migration1 = []string{
	`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`CREATE TABLE users (
		user_id INTEGER PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT,
		email_confirmed_at TEXT,
		status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','live','suspended')),
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`CREATE TABLE user_configs (
		user_id INTEGER PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
		resume_text TEXT,
		experience_text TEXT,
		desired_titles_json TEXT NOT NULL DEFAULT '[]',
		search_preferences_json TEXT NOT NULL DEFAULT '{}',
		blocked_companies_json TEXT NOT NULL DEFAULT '[]',
		matcher_name TEXT,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`CREATE TABLE user_history (
		user_history_id INTEGER PRIMARY KEY,
		user_id INTEGER REFERENCES users(user_id) ON DELETE SET NULL,
		event_type TEXT NOT NULL,
		inbound_ip TEXT,
		data_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`CREATE TABLE job_boards (
		board_id INTEGER PRIMARY KEY,
		board_url TEXT NOT NULL UNIQUE,
		company TEXT NOT NULL,
		ats_type TEXT,
		status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','unsupported')),
		metadata_json TEXT NOT NULL DEFAULT '{}',
		discovered_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`CREATE TABLE job_listings (
		job_listing_id INTEGER PRIMARY KEY,
		canonical_url TEXT NOT NULL UNIQUE,
		board_id INTEGER NOT NULL REFERENCES job_boards(board_id) ON DELETE RESTRICT,
		current_version INTEGER NOT NULL DEFAULT 0,
		last_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		posted_at TEXT,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`CREATE TABLE job_listing_versions (
		job_listing_id INTEGER NOT NULL REFERENCES job_listings(job_listing_id) ON DELETE CASCADE,
		version INTEGER NOT NULL,
		found_at TEXT NOT NULL,
		posted_at TEXT,
		title TEXT NOT NULL,
		salary TEXT NOT NULL DEFAULT '',
		body_text TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		PRIMARY KEY (job_listing_id, version),
		UNIQUE (job_listing_id, content_hash)
	)`,
	`CREATE TABLE job_matches (
		job_match_id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
		job_listing_id INTEGER NOT NULL REFERENCES job_listings(job_listing_id) ON DELETE CASCADE,
		job_listing_version INTEGER NOT NULL,
		rating INTEGER CHECK (rating BETWEEN 1 AND 5),
		decision TEXT NOT NULL DEFAULT 'deferred' CHECK (decision IN ('match','no_match','failure','deferred')),
		matcher_name TEXT NOT NULL,
		matcher_metadata_json TEXT NOT NULL DEFAULT '{}',
		job_match_data_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		UNIQUE (user_id, job_listing_id, job_listing_version, matcher_name),
		FOREIGN KEY (job_listing_id, job_listing_version) REFERENCES job_listing_versions(job_listing_id, version)
	)`,
	`CREATE TABLE job_history (
		job_history_id INTEGER PRIMARY KEY,
		service TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT,
		status TEXT NOT NULL CHECK (status IN ('success','failure','partial')),
		items_processed INTEGER NOT NULL DEFAULT 0,
		data_json TEXT NOT NULL DEFAULT '{}',
		error_json TEXT,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`INSERT INTO schema_migrations(version) VALUES (1)`,
}

var migration2 = []string{
	`CREATE TABLE job_listing_versions_v2 (
		job_listing_id INTEGER NOT NULL REFERENCES job_listings(job_listing_id) ON DELETE CASCADE,
		version INTEGER NOT NULL,
		found_at TEXT NOT NULL,
		posted_at TEXT,
		title TEXT NOT NULL,
		salary TEXT NOT NULL DEFAULT '',
		body_text TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		PRIMARY KEY (job_listing_id, version)
	)`,
	`INSERT INTO job_listing_versions_v2 (job_listing_id, version, found_at, posted_at, title, salary, body_text, content_hash)
		SELECT job_listing_id, version, found_at, posted_at, title, salary, body_text, content_hash FROM job_listing_versions`,
	`DROP TABLE job_listing_versions`,
	`ALTER TABLE job_listing_versions_v2 RENAME TO job_listing_versions`,
	`CREATE INDEX IF NOT EXISTS idx_job_boards_status ON job_boards(status)`,
	`CREATE INDEX IF NOT EXISTS idx_job_listing_versions_hash ON job_listing_versions(content_hash)`,
	`CREATE INDEX IF NOT EXISTS idx_job_matches_user_created ON job_matches(user_id, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_job_history_service_started ON job_history(service, started_at)`,
	`INSERT INTO schema_migrations(version) VALUES (2)`,
}

var migration3 = []string{
	`DROP INDEX IF EXISTS idx_job_boards_status`,
	`CREATE TABLE job_boards_v3 (
		board_id INTEGER PRIMARY KEY,
		board_url TEXT NOT NULL UNIQUE,
		company TEXT NOT NULL,
		ats_type TEXT,
		status TEXT NOT NULL DEFAULT 'discovered' CHECK (status IN ('discovered','active','unsupported')),
		metadata_json TEXT NOT NULL DEFAULT '{}',
		discovered_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`INSERT INTO job_boards_v3 (board_id, board_url, company, ats_type, status, metadata_json, discovered_at, updated_at)
		SELECT board_id, board_url, company, ats_type, status, metadata_json, discovered_at, updated_at FROM job_boards`,
	`DROP TABLE job_boards`,
	`ALTER TABLE job_boards_v3 RENAME TO job_boards`,
	`CREATE INDEX IF NOT EXISTS idx_job_boards_status ON job_boards(status)`,
	`INSERT INTO schema_migrations(version) VALUES (3)`,
}

var migration4 = []string{
	`CREATE TABLE site_config (
		config_id INTEGER PRIMARY KEY CHECK (config_id = 1),
		signups_disabled INTEGER NOT NULL DEFAULT 0 CHECK (signups_disabled IN (0, 1)),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`INSERT INTO site_config(config_id, signups_disabled) VALUES (1, 0)`,
	// Tombstoned users are represented as a distinct users.status enum value.
	// Related rows keep the existing users foreign-key behavior because user ids
	// remain stable while only the users CHECK constraint is widened.
	`CREATE TABLE users_v4 (
		user_id INTEGER PRIMARY KEY,
		email TEXT NOT NULL,
		password_hash TEXT,
		email_confirmed_at TEXT,
		status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','live','suspended','tombstoned')),
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`INSERT INTO users_v4 (user_id, email, password_hash, email_confirmed_at, status, created_at, updated_at)
		SELECT user_id, email, password_hash, email_confirmed_at, status, created_at, updated_at FROM users`,
	`DROP TABLE users`,
	`ALTER TABLE users_v4 RENAME TO users`,
	`CREATE UNIQUE INDEX idx_users_email_active ON users(email) WHERE status <> 'tombstoned'`,
	`INSERT INTO schema_migrations(version) VALUES (4)`,
}

type ListingVersionInput struct {
	CanonicalURL string
	BoardID      int64
	Title        string
	Salary       string
	Body         string
	FoundAt      time.Time
	PostedAt     sql.NullTime
	MetadataJSON string
}

type ListingVersionResult struct {
	ListingID int64
	Version   int
	Inserted  bool
}

func UpsertListingVersion(ctx context.Context, conn *sql.DB, in ListingVersionInput) (ListingVersionResult, error) {
	if in.CanonicalURL == "" || in.BoardID == 0 || in.Title == "" {
		return ListingVersionResult{}, errors.New("canonical URL, board ID, and title are required")
	}
	if in.FoundAt.IsZero() {
		in.FoundAt = time.Now().UTC()
	}
	if in.MetadataJSON == "" {
		in.MetadataJSON = "{}"
	}
	body := schema.NormalizeBodyText(in.Body)
	hash := schema.ListingContentHash(in.Title, in.Salary, in.Body)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return ListingVersionResult{}, err
	}
	defer tx.Rollback()

	var posted interface{}
	if in.PostedAt.Valid {
		posted = in.PostedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO job_listings (canonical_url, board_id, last_seen_at, posted_at, metadata_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(canonical_url) DO UPDATE SET
			board_id = excluded.board_id,
			last_seen_at = excluded.last_seen_at,
			posted_at = excluded.posted_at,
			metadata_json = excluded.metadata_json,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`, in.CanonicalURL, in.BoardID, in.FoundAt.UTC().Format(time.RFC3339Nano), posted, in.MetadataJSON); err != nil {
		return ListingVersionResult{}, err
	}
	var listingID int64
	var currentVersion int
	if err := tx.QueryRowContext(ctx, `SELECT job_listing_id, current_version FROM job_listings WHERE canonical_url = ?`, in.CanonicalURL).Scan(&listingID, &currentVersion); err != nil {
		return ListingVersionResult{}, err
	}
	var latestHash sql.NullString
	if currentVersion > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT content_hash FROM job_listing_versions WHERE job_listing_id = ? AND version = ?`, listingID, currentVersion).Scan(&latestHash); err != nil {
			return ListingVersionResult{}, err
		}
	}
	if latestHash.Valid && latestHash.String == hash {
		return ListingVersionResult{ListingID: listingID, Version: currentVersion, Inserted: false}, tx.Commit()
	}
	nextVersion := currentVersion + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO job_listing_versions (job_listing_id, version, found_at, posted_at, title, salary, body_text, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, listingID, nextVersion, in.FoundAt.UTC().Format(time.RFC3339Nano), posted, in.Title, in.Salary, body, hash); err != nil {
		return ListingVersionResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE job_listings SET current_version = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE job_listing_id = ?`, nextVersion, listingID); err != nil {
		return ListingVersionResult{}, err
	}
	return ListingVersionResult{ListingID: listingID, Version: nextVersion, Inserted: true}, tx.Commit()
}
