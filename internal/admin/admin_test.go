package admin

import (
	"context"
	"testing"
	"time"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/schema"
)

func newTestStore(t *testing.T) Store {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx := context.Background()
	if err := db.EnsureCurrent(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAdminTables(ctx, conn); err != nil {
		t.Fatal(err)
	}
	return Store{DB: conn}
}

func TestApproveAndSuspendUser(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	res, err := store.DB.ExecContext(ctx, `INSERT INTO users (email) VALUES ('person@example.com')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if err := store.ApproveUser(ctx, id, "admin@example.com"); err != nil {
		t.Fatalf("ApproveUser: %v", err)
	}
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := users[0].Status; got != schema.UserStatusLive {
		t.Fatalf("approved status = %q", got)
	}
	if err := store.SuspendUser(ctx, id, "admin@example.com"); err != nil {
		t.Fatalf("SuspendUser: %v", err)
	}
	users, _ = store.ListUsers(ctx)
	if got := users[0].Status; got != schema.UserStatusSuspended {
		t.Fatalf("suspended status = %q", got)
	}
}

func TestIssueValidateAndRevokeToken(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	issued, err := store.IssueToken(ctx, "deploy", "ops@example.com", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if issued.Token == "" {
		t.Fatal("token was not returned")
	}
	if _, err := store.ValidateToken(ctx, issued.Token); err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if err := store.RevokeToken(ctx, issued.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if _, err := store.ValidateToken(ctx, issued.Token); err != ErrTokenRevoked {
		t.Fatalf("ValidateToken after revoke = %v, want ErrTokenRevoked", err)
	}
}

func TestProcessHistory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_, err := store.DB.ExecContext(ctx, `INSERT INTO job_history (service, started_at, finished_at, status, items_processed, error_json) VALUES
		('board-search', '2025-01-01T00:00:00Z', '2025-01-01T00:01:00Z', 'success', 10, NULL),
		('match-search', '2025-01-02T00:00:00Z', '2025-01-02T00:01:00Z', 'failure', 3, '{"error":"classifier unavailable"}')`)
	if err != nil {
		t.Fatal(err)
	}
	history, counts, err := store.ProcessHistory(ctx, 10)
	if err != nil {
		t.Fatalf("ProcessHistory: %v", err)
	}
	if len(history) != 2 || history[0].Service != "match-search" || !history[0].ErrorJSON.Valid {
		t.Fatalf("history rows not ordered with error detail: %#v", history)
	}
	if len(counts) != 2 {
		t.Fatalf("counts = %#v", counts)
	}
}
