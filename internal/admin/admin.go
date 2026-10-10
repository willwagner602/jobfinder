package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"jobs.willwagner.me/internal/schema"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrTokenRevoked = errors.New("token revoked")
)

// Store contains admin operations that are intentionally limited to the
// shared database contract plus the admin service's token table.
type Store struct {
	DB *sql.DB
}

type User struct {
	ID               int64
	Email            string
	Status           schema.UserStatus
	EmailConfirmedAt sql.NullString
	CreatedAt        string
	UpdatedAt        string
}

type IssuedToken struct {
	ID        int64
	Token     string
	Subject   string
	IssuedTo  string
	ExpiresAt time.Time
}

type StoredToken struct {
	ID        int64
	Subject   string
	IssuedTo  string
	ExpiresAt string
	RevokedAt sql.NullString
	CreatedAt string
}

type HistoryRow struct {
	ID             int64
	Service        string
	StartedAt      string
	FinishedAt     sql.NullString
	Status         schema.ProcessStatus
	ItemsProcessed int64
	DataJSON       string
	ErrorJSON      sql.NullString
}

type StatusCount struct {
	Status schema.ProcessStatus
	Count  int64
}

func EnsureAdminTables(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS admin_tokens (
		admin_token_id INTEGER PRIMARY KEY,
		token_hash TEXT NOT NULL UNIQUE,
		subject TEXT NOT NULL,
		issued_to TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		revoked_at TEXT,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`)
	return err
}

func (s Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id, email, status, email_confirmed_at, created_at, updated_at FROM users ORDER BY created_at DESC, user_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Status, &u.EmailConfirmedAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s Store) ApproveUser(ctx context.Context, userID int64, adminEmail string) error {
	return s.setUserStatus(ctx, userID, schema.UserStatusLive, "admin_approved_user", adminEmail)
}

func (s Store) SuspendUser(ctx context.Context, userID int64, adminEmail string) error {
	return s.setUserStatus(ctx, userID, schema.UserStatusSuspended, "admin_suspended_user", adminEmail)
}

func (s Store) setUserStatus(ctx context.Context, userID int64, status schema.UserStatus, eventType, adminEmail string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE users SET status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE user_id = ?`, status, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_history (user_id, event_type, data_json) VALUES (?, ?, ?)`, userID, eventType, fmt.Sprintf(`{"admin_email":%q,"status":%q}`, adminEmail, status)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s Store) IssueToken(ctx context.Context, subject, issuedTo string, ttl time.Duration) (IssuedToken, error) {
	if subject == "" || issuedTo == "" {
		return IssuedToken{}, errors.New("subject and issued recipient are required")
	}
	if ttl <= 0 {
		return IssuedToken{}, errors.New("ttl must be positive")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return IssuedToken{}, err
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().UTC().Add(ttl)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO admin_tokens (token_hash, subject, issued_to, expires_at) VALUES (?, ?, ?, ?)`, hashToken(token), subject, issuedTo, expires.Format(time.RFC3339Nano))
	if err != nil {
		return IssuedToken{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return IssuedToken{}, err
	}
	return IssuedToken{ID: id, Token: token, Subject: subject, IssuedTo: issuedTo, ExpiresAt: expires}, nil
}

func (s Store) RevokeToken(ctx context.Context, tokenID int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE admin_tokens SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE admin_token_id = ? AND revoked_at IS NULL`, tokenID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s Store) ValidateToken(ctx context.Context, token string) (StoredToken, error) {
	var t StoredToken
	err := s.DB.QueryRowContext(ctx, `SELECT admin_token_id, subject, issued_to, expires_at, revoked_at, created_at FROM admin_tokens WHERE token_hash = ?`, hashToken(token)).Scan(&t.ID, &t.Subject, &t.IssuedTo, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredToken{}, ErrNotFound
	}
	if err != nil {
		return StoredToken{}, err
	}
	if t.RevokedAt.Valid {
		return StoredToken{}, ErrTokenRevoked
	}
	expires, err := time.Parse(time.RFC3339Nano, t.ExpiresAt)
	if err != nil {
		return StoredToken{}, err
	}
	if time.Now().UTC().After(expires) {
		return StoredToken{}, ErrTokenRevoked
	}
	return t, nil
}

func (s Store) ListTokens(ctx context.Context) ([]StoredToken, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT admin_token_id, subject, issued_to, expires_at, revoked_at, created_at FROM admin_tokens ORDER BY created_at DESC, admin_token_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []StoredToken
	for rows.Next() {
		var t StoredToken
		if err := rows.Scan(&t.ID, &t.Subject, &t.IssuedTo, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s Store) ProcessHistory(ctx context.Context, limit int) ([]HistoryRow, []StatusCount, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT job_history_id, service, started_at, finished_at, status, items_processed, data_json, error_json FROM job_history ORDER BY started_at DESC, job_history_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var history []HistoryRow
	for rows.Next() {
		var h HistoryRow
		if err := rows.Scan(&h.ID, &h.Service, &h.StartedAt, &h.FinishedAt, &h.Status, &h.ItemsProcessed, &h.DataJSON, &h.ErrorJSON); err != nil {
			return nil, nil, err
		}
		history = append(history, h)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	countRows, err := s.DB.QueryContext(ctx, `SELECT status, count(*) FROM job_history GROUP BY status ORDER BY status`)
	if err != nil {
		return nil, nil, err
	}
	defer countRows.Close()
	var counts []StatusCount
	for countRows.Next() {
		var c StatusCount
		if err := countRows.Scan(&c.Status, &c.Count); err != nil {
			return nil, nil, err
		}
		counts = append(counts, c)
	}
	return history, counts, countRows.Err()
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
