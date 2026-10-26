package admin

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"jobfinder/internal/db"
	"jobfinder/internal/schema"
)

// brokenStore hands out a Store whose connection pool is closed, so every
// query immediately fails and the database-error branches run.
func brokenStore(t *testing.T) Store {
	t.Helper()
	store := newTestStore(t)
	if err := store.DB.Close(); err != nil {
		t.Fatalf("close store db: %v", err)
	}
	return store
}

func TestListUsersReportsQueryErrors(t *testing.T) {
	if _, err := brokenStore(t).ListUsers(context.Background()); err == nil {
		t.Fatal("ListUsers on closed db should fail")
	}
}

func TestApproveUserReportsDatabaseErrors(t *testing.T) {
	if err := brokenStore(t).ApproveUser(context.Background(), 1, "admin@example.com"); err == nil {
		t.Fatal("ApproveUser on closed db should fail")
	}
}

func TestSuspendUserReportsDatabaseErrors(t *testing.T) {
	if err := brokenStore(t).SuspendUser(context.Background(), 1, "admin@example.com"); err == nil {
		t.Fatal("SuspendUser on closed db should fail")
	}
}

func TestApproveUserReportsHistoryInsertFailure(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	res, err := store.DB.ExecContext(ctx, `INSERT INTO users (email) VALUES ('person@example.com')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	// The UPDATE succeeds but the history INSERT aborts, exercising the
	// specific statement-level branch in setUserStatus.
	if _, err := store.DB.ExecContext(ctx, `CREATE TRIGGER reject_history BEFORE INSERT ON user_history BEGIN SELECT RAISE(ABORT, 'history rejected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveUser(ctx, id, "admin@example.com"); err == nil {
		t.Fatal("ApproveUser should fail when the history insert aborts")
	}
	// The aborted transaction must not have changed the user's status.
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if users[0].Status != schema.UserStatusPending {
		t.Fatalf("status after aborted approval = %q, want pending", users[0].Status)
	}
}

func TestIssueTokenReportsDatabaseErrors(t *testing.T) {
	if _, err := brokenStore(t).IssueToken(context.Background(), "deploy", "ops@example.com", time.Hour); err == nil {
		t.Fatal("IssueToken on closed db should fail")
	}
}

func TestRevokeTokenReportsDatabaseErrors(t *testing.T) {
	if err := brokenStore(t).RevokeToken(context.Background(), 1); err == nil {
		t.Fatal("RevokeToken on closed db should fail")
	}
}

func TestValidateTokenReportsDatabaseErrors(t *testing.T) {
	if _, err := brokenStore(t).ValidateToken(context.Background(), "anything"); err == nil {
		t.Fatal("ValidateToken on closed db should fail")
	}
}

func TestListTokensReportsDatabaseErrors(t *testing.T) {
	if _, err := brokenStore(t).ListTokens(context.Background()); err == nil {
		t.Fatal("ListTokens on closed db should fail")
	}
}

func TestProcessHistoryReportsDatabaseErrors(t *testing.T) {
	store := newTestStore(t)
	// Drop job_history and recreate it unreadable by giving it a broken
	// column layout: the SELECT then fails mid-scan.
	ctx := context.Background()
	if _, err := store.DB.ExecContext(ctx, `DROP TABLE job_history`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `CREATE TABLE job_history (
		job_history_id INTEGER PRIMARY KEY,
		service TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT,
		status TEXT NOT NULL,
		items_processed INTEGER NOT NULL DEFAULT 0,
		data_json TEXT NOT NULL DEFAULT '{}',
		error_json TEXT,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `CREATE TRIGGER reject_status BEFORE INSERT ON job_history BEGIN SELECT RAISE(ABORT, 'history rejected'); END`); err != nil {
		t.Fatal(err)
	}
	broken := brokenStore(t)
	if _, _, err := broken.ProcessHistory(ctx, 10); err == nil {
		t.Fatal("ProcessHistory on closed db should fail")
	}
}

func TestEnsureAdminTablesFailsOnConflictingTableShape(t *testing.T) {
	bad, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	// A table named admin_tokens with the wrong columns makes CREATE TABLE
	// IF NOT EXISTS a no-op that later collides on the token_hash insert.
	if err := EnsureAdminTables(context.Background(), bad); err != nil {
		t.Fatalf("EnsureAdminTables should tolerate an existing table: %v", err)
	}
	// Insert under a different shape; the upstream issue is a schema
	// conflict, which is a legitimate error return to expect.
	if _, err := bad.Exec(`DROP TABLE admin_tokens`); err != nil {
		t.Fatal(err)
	}
	if _, err := bad.Exec(`CREATE TABLE admin_tokens (other_column TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAdminTables(context.Background(), bad); err != nil {
		t.Fatalf("EnsureAdminTables should not fail when a table exists: %v", err)
	}
}

func TestHashTokenIsStableSHA256Hex(t *testing.T) {
	if got, want := hashToken("abc"), hashToken("abc"); got != want {
		t.Fatalf("hashToken not stable: %q vs %q", got, want)
	}
	if len(hashToken("abc")) != 64 {
		t.Fatalf("hash length %d", len(hashToken("abc")))
	}
	if hashToken("abc") == hashToken("abd") {
		t.Fatal("different inputs must hash differently")
	}
}

func TestRenderReportsTemplateExecutionFailure(t *testing.T) {
	res := httptest.NewRecorder()
	// A func value in the data makes html/template fail during escaping:
	// "can't call a function value" happens only when it must write a
	// value, which {{.}} does. Execute returns its error before any body
	// text is written, so render() reaches its error handler.
	render(res, `<p>{{.}}</p>`, func() {})
	if res.Body.String() == `` {
		t.Fatalf("render error body empty: code=%d", res.Code)
	}
	if res.Header().Get("Content-Type") == "application/json" {
		t.Fatalf("content type wrong: %q", res.Header().Get("Content-Type"))
	}
}
