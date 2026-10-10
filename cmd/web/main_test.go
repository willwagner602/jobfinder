package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jobs.willwagner.me/internal/db"
)

func TestRunFailsWithoutSMTPConfig(t *testing.T) {
	t.Setenv("JOBS_SMTP_ADDR", "")
	t.Setenv("JOBS_SMTP_USERNAME", "")
	t.Setenv("JOBS_SMTP_PASSWORD", "")
	if err := run(); err == nil || !strings.Contains(err.Error(), "JOBS_SMTP_ADDR") {
		t.Fatalf("run() without SMTP config err = %v, want missing JOBS_SMTP_ADDR", err)
	}
}

func TestRunFailsWithoutSessionKeyConfig(t *testing.T) {
	t.Setenv("JOBS_SMTP_ADDR", "127.0.0.1:2525")
	t.Setenv("JOBS_SMTP_USERNAME", "u")
	t.Setenv("JOBS_SMTP_PASSWORD", "p")
	t.Setenv("SESSION_KEY", "short")
	if err := run(); err == nil || !strings.Contains(err.Error(), "SESSION_KEY") {
		t.Fatalf("run() with short SESSION_KEY err = %v", err)
	}
}

func TestRunFailsWhenDatabaseCannotBeOpened(t *testing.T) {
	t.Setenv("JOBS_SMTP_ADDR", "127.0.0.1:2525")
	t.Setenv("JOBS_SMTP_USERNAME", "u")
	t.Setenv("JOBS_SMTP_PASSWORD", "p")
	t.Setenv("JOBS_DB", t.TempDir()) // a directory is not a usable sqlite file
	if err := run(); err == nil {
		t.Fatal("run() should fail when the database path is not a usable sqlite file")
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("JOBS_TEST_UNSET", "")
	os.Unsetenv("JOBS_TEST_UNSET")
	if got := env("JOBS_TEST_UNSET", "fallback"); got != "fallback" {
		t.Fatalf("unset env value = %q, want fallback", got)
	}
	t.Setenv("JOBS_TEST_SET", "configured")
	if got := env("JOBS_TEST_SET", "fallback"); got != "configured" {
		t.Fatalf("configured env value = %q, want %q", got, "configured")
	}
}

func TestSessionKeyValidation(t *testing.T) {
	t.Setenv("SESSION_KEY", "")
	os.Unsetenv("SESSION_KEY")
	key, err := sessionKey()
	if err != nil {
		t.Fatalf("generated session key: %v", err)
	}
	if len(key) < 32 {
		t.Fatalf("generated key len = %d, want >= 32", len(key))
	}
	t.Setenv("SESSION_KEY", "short")
	if _, err := sessionKey(); err == nil {
		t.Fatal("short SESSION_KEY accepted")
	}
	t.Setenv("SESSION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef")
	if key, err := sessionKey(); err != nil || len(key) < 32 {
		t.Fatalf("plain-text SESSION_KEY rejected: len=%d err=%v", len(key), err)
	}
	t.Setenv("SESSION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if key, err := sessionKey(); err != nil || len(key) < 32 {
		t.Fatalf("base64 SESSION_KEY rejected: len=%d err=%v", len(key), err)
	}
}

func TestRunAppliesSchemaAndServesRoutes(t *testing.T) {
	addr := freePort(t)
	t.Setenv("WEB_ADDR", addr)
	t.Setenv("JOBS_DB", filepath.Join(t.TempDir(), "jobs.sqlite3"))
	t.Setenv("TEMPLATE_DIR", "../../templates")
	t.Setenv("SESSION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("JOBS_SMTP_ADDR", "127.0.0.1:2525")
	t.Setenv("JOBS_SMTP_USERNAME", "tester")
	t.Setenv("JOBS_SMTP_PASSWORD", "testpass")

	done := make(chan error, 1)
	go func() { done <- run() }()

	if err := waitHealthy(t, addr); err != nil {
		select {
		case runErr := <-done:
			t.Fatalf("run() exited before becoming healthy: %v", runErr)
		default:
		}
		t.Fatal(err)
	}

	conn, err := sql.Open("sqlite3", os.Getenv("JOBS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	version, err := db.Version(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if version < 2 {
		t.Fatalf("schema version = %d, want >= 2 after run() startup", version)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	return addr
}

func waitHealthy(t *testing.T, addr string) error {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusNoContent {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("service did not become healthy at %s", addr)
}

var _ = httptest.NewRequest // keep httptest import; helper may be used by future route tests
var _ = func() error { return run() }
