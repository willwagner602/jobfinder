package admin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"jobfinder/internal/db"
	"jobfinder/internal/schema"
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

func TestSignupDisabledRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	disabled, err := store.IsSignupDisabled(ctx)
	if err != nil {
		t.Fatalf("IsSignupDisabled: %v", err)
	}
	if disabled {
		t.Fatal("fresh database should allow signups")
	}
	if err := store.SetSignupDisabled(ctx, true); err != nil {
		t.Fatalf("SetSignupDisabled(true): %v", err)
	}
	disabled, err = store.IsSignupDisabled(ctx)
	if err != nil {
		t.Fatalf("IsSignupDisabled after set: %v", err)
	}
	if !disabled {
		t.Fatal("signups_disabled was not persisted")
	}
	if err := store.SetSignupDisabled(ctx, false); err != nil {
		t.Fatalf("SetSignupDisabled(false): %v", err)
	}
	disabled, err = store.IsSignupDisabled(ctx)
	if err != nil {
		t.Fatalf("IsSignupDisabled after clear: %v", err)
	}
	if disabled {
		t.Fatal("signups_disabled was not cleared")
	}
}

func TestTombstoneAndUnTombstoneUserPreserveRelatedRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	res, err := store.DB.ExecContext(ctx, `INSERT INTO users (email, status) VALUES ('person@example.com', 'live')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO user_configs(user_id, resume_text) VALUES (?, 'resume')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO job_boards(board_id, board_url, company, ats_type, status) VALUES (1, 'https://example.com/jobs', 'Example', 'greenhouse', 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO job_listings(job_listing_id, canonical_url, board_id, current_version, last_seen_at) VALUES (1, 'https://example.com/jobs/1', 1, 1, '2025-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO job_listing_versions(job_listing_id, version, found_at, title, body_text, content_hash) VALUES (1, 1, '2025-01-01T00:00:00Z', 'Engineer', 'Body', 'hash')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO job_matches(user_id, job_listing_id, job_listing_version, matcher_name) VALUES (?, 1, 1, 'fixture')`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.TombstoneUser(ctx, id, "admin@example.com"); err != nil {
		t.Fatalf("TombstoneUser: %v", err)
	}
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if users[0].Status != schema.UserStatusTombstoned {
		t.Fatalf("tombstoned status = %q", users[0].Status)
	}
	if err := store.UnTombstoneUser(ctx, id, "admin@example.com"); err != nil {
		t.Fatalf("UnTombstoneUser: %v", err)
	}
	users, err = store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if users[0].Status != schema.UserStatusSuspended {
		t.Fatalf("untombstoned status = %q, want suspended", users[0].Status)
	}
	for table, query := range map[string]string{
		"user_configs": `SELECT count(*) FROM user_configs WHERE user_id = ?`,
		"job_matches":  `SELECT count(*) FROM job_matches WHERE user_id = ?`,
		"history":      `SELECT count(*) FROM user_history WHERE user_id = ? AND event_type IN ('admin_tombstoned_user', 'admin_untombstoned_user')`,
	} {
		var rows int
		if err := store.DB.QueryRowContext(ctx, query, id).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		want := 1
		if table == "history" {
			want = 2
		}
		if rows != want {
			t.Fatalf("%s rows = %d, want %d", table, rows, want)
		}
	}
}

func TestTombstoneUnknownUserIsNotFoundWithoutHistory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.TombstoneUser(ctx, 1234, "admin@example.com"); err != ErrNotFound {
		t.Fatalf("TombstoneUser on unknown id = %v, want ErrNotFound", err)
	}
	if err := store.UnTombstoneUser(ctx, 1234, "admin@example.com"); err != ErrNotFound {
		t.Fatalf("UnTombstoneUser on unknown id = %v, want ErrNotFound", err)
	}
	var events int
	if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM user_history`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatalf("aborted tombstone change wrote %d history rows", events)
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
	if counts[0].Status != schema.ProcessStatus("failure") || counts[0].Count != 1 {
		t.Fatalf("counts[0] = %#v", counts[0])
	}
	if counts[1].Status != schema.ProcessStatus("success") || counts[1].Count != 1 {
		t.Fatalf("counts[1] = %#v", counts[1])
	}
}

func TestProcessHistoryDefaultLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := store.DB.ExecContext(ctx, `INSERT INTO job_history (service, started_at, status) VALUES ('svc', '2025-01-01T00:00:00Z', 'partial')`); err != nil {
			t.Fatal(err)
		}
	}
	for _, limit := range []int{-1, 0, 501} {
		history, counts, err := store.ProcessHistory(ctx, limit)
		if err != nil {
			t.Fatalf("ProcessHistory(%d): %v", limit, err)
		}
		if len(history) != 3 {
			t.Fatalf("history rows = %d, want 3", len(history))
		}
		if len(counts) != 1 || counts[0].Status != schema.ProcessStatus("partial") || counts[0].Count != 3 {
			t.Fatalf("counts = %#v", counts)
		}
	}
}

func TestSetUserStatusUnknownUserIsNotFoundWithoutHistory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.ApproveUser(ctx, 1234, "admin@example.com"); err != ErrNotFound {
		t.Fatalf("ApproveUser on unknown id = %v, want ErrNotFound", err)
	}
	if err := store.SuspendUser(ctx, 1234, "admin@example.com"); err != ErrNotFound {
		t.Fatalf("SuspendUser on unknown id = %v, want ErrNotFound", err)
	}
	var events int
	if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM user_history`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatalf("aborted status change wrote %d history rows", events)
	}
}

func TestSetUserStatusIsIdempotentAndRecordsHistoryEvents(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	res, err := store.DB.ExecContext(ctx, `INSERT INTO users (email, status) VALUES ('person@example.com', 'pending')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	// Approving twice must not fail; each transition is a recorded event.
	if err := store.ApproveUser(ctx, id, "admin@example.com"); err != nil {
		t.Fatalf("ApproveUser: %v", err)
	}
	if err := store.ApproveUser(ctx, id, "other-admin@example.com"); err != nil {
		t.Fatalf("ApproveUser again: %v", err)
	}
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if users[0].Status != schema.UserStatusLive {
		t.Fatalf("status = %q, want live", users[0].Status)
	}
	var events int
	if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM user_history WHERE user_id = ? AND event_type = 'admin_approved_user'`, id).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("approval history events = %d, want 2 (one per transition)", events)
	}
}

func TestIssueTokenRejectsMissingSubjectOrRecipientAndZeroTTL(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.IssueToken(ctx, "", "ops@example.com", time.Hour); err == nil {
		t.Fatal("empty subject should be rejected")
	}
	if _, err := store.IssueToken(ctx, "deploy", "", time.Hour); err == nil {
		t.Fatal("empty issued_to should be rejected")
	}
	if _, err := store.IssueToken(ctx, "deploy", "ops@example.com", 0); err == nil {
		t.Fatal("zero ttl should be rejected")
	}
	if _, err := store.IssueToken(ctx, "deploy", "ops@example.com", -time.Hour); err == nil {
		t.Fatal("negative ttl should be rejected")
	}
	var tokens int
	if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM admin_tokens`).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if tokens != 0 {
		t.Fatalf("rejected issues persisted %d rows", tokens)
	}
}

func TestIssueTokenStoresHashAndUniqueSubjectValues(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	first, err := store.IssueToken(ctx, "deploy", "ops@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.IssueToken(ctx, "deploy", "ops@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Fatal("two issued tokens must differ")
	}
	if first.ExpiresAt.IsZero() || !first.ExpiresAt.After(time.Now().UTC().Add(-time.Minute)) {
		t.Fatalf("issued expiry %v is in the past", first.ExpiresAt)
	}
	var gotHash string
	if err := store.DB.QueryRowContext(ctx, `SELECT token_hash FROM admin_tokens WHERE admin_token_id = ?`, first.ID).Scan(&gotHash); err != nil {
		t.Fatal(err)
	}
	if gotHash != hashToken(first.Token) {
		t.Fatalf("stored hash mismatch: %q vs %q", gotHash, hashToken(first.Token))
	}
	if err := store.RevokeToken(ctx, second.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if err := store.RevokeToken(ctx, second.ID); err != ErrNotFound {
		t.Fatalf("second RevokeToken = %v, want ErrNotFound", err)
	}
	if err := store.RevokeToken(ctx, 99999); err != ErrNotFound {
		t.Fatalf("RevokeToken on unknown id = %v, want ErrNotFound", err)
	}
}

func TestValidateTokenRejectsUnknownHashAndExpiredTokens(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.ValidateToken(ctx, "unknown-plain-value"); err != ErrNotFound {
		t.Fatalf("ValidateToken unknown = %v, want ErrNotFound", err)
	}
	issued, err := store.IssueToken(ctx, "deploy", "ops@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeToken(ctx, issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidateToken(ctx, issued.Token); err != ErrTokenRevoked {
		t.Fatalf("ValidateToken revoked = %v, want ErrTokenRevoked", err)
	}
	// A token whose stored expiry is already in the past is treated as
	// revoked at validation time.
	if _, err := store.DB.ExecContext(ctx, `UPDATE admin_tokens SET revoked_at = NULL, expires_at = '2020-01-01T00:00:00Z' WHERE admin_token_id = ?`, issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidateToken(ctx, issued.Token); err != ErrTokenRevoked {
		t.Fatalf("ValidateToken expired = %v, want ErrTokenRevoked", err)
	}
}

func TestValidateTokenRejectsUnparseableExpiry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	issued, err := store.IssueToken(ctx, "deploy", "ops@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE admin_tokens SET expires_at = 'not-a-timestamp' WHERE admin_token_id = ?`, issued.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidateToken(ctx, issued.Token); err == nil {
		t.Fatal("unparseable expires_at should fail validation")
	}
}

func TestListTokensOrdersNewestFirst(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := store.IssueToken(ctx, fmt.Sprintf("subject-%d", i), "ops@example.com", time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	tokens, err := store.ListTokens(ctx)
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("tokens = %#v", tokens)
	}
	if tokens[0].Subject != "subject-2" {
		t.Fatalf("newest token subject = %q, want subject-2", tokens[0].Subject)
	}
	if tokens[0].RevokedAt.Valid {
		t.Fatal("fresh token must not be revoked")
	}
}
