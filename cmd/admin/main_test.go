package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("JOBS_TEST_ENV_VAR", "")
	if got := envOrDefault("JOBS_TEST_ENV_VAR", "fallback"); got != "fallback" {
		t.Fatalf("empty env value = %q, want fallback %q", got, "fallback")
	}
	if got := envOrDefault("JOBS_TEST_UNSET_VAR", "fallback"); got != "fallback" {
		t.Fatalf("unset env value = %q, want fallback %q", got, "fallback")
	}
	t.Setenv("JOBS_TEST_ENV_VAR", "configured")
	if got := envOrDefault("JOBS_TEST_ENV_VAR", "fallback"); got != "configured" {
		t.Fatalf("configured env value = %q, want %q", got, "configured")
	}
}

// resetStartupEnv points run() at a temp database, a free port, and a
// bootstrap admin token like the deployed service expects.
func resetStartupEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JOBS_ADMIN_ADDR", "127.0.0.1:0")
	t.Setenv("JOBS_DB_PATH", filepath.Join(t.TempDir(), "jobs.sqlite3"))
	t.Setenv("JOBS_ADMIN_TOKEN", "test-bootstrap-token")
}

func TestRunFailsWhenDatabaseCannotBeOpened(t *testing.T) {
	resetStartupEnv(t)
	// A directory in place of the sqlite file makes db.Open fail on first use.
	t.Setenv("JOBS_DB_PATH", t.TempDir()) // a directory is not a usable sqlite file
	if err := run(); err == nil {
		t.Fatal("run() should fail when the database path is not a usable sqlite file")
	}
}

func TestRunFailsWhenSchemaIsNewerThanApplication(t *testing.T) {
	resetStartupEnv(t)
	dbPath := os.Getenv("JOBS_DB_PATH")
	conn, err := sql.Open("sqlite3", dbPath) // driver registered via internal/db import
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO schema_migrations (version) VALUES (999)`); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	err = run()
	if err == nil || !strings.Contains(err.Error(), "newer than application schema version") {
		t.Fatalf("run() should reject a newer schema, got err=%v", err)
	}
}

func TestRunAppliesSchemaAndServesRoutes(t *testing.T) {
	resetStartupEnv(t)
	// Pick a concrete free port and hand it over so the health probe can
	// address the service.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	t.Setenv("JOBS_ADMIN_ADDR", addr)

	done := make(chan error, 1)
	go func() { done <- run() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := waitHealthy(ctx, addr); err != nil {
		select {
		case runErr := <-done:
			t.Fatalf("run() exited before becoming healthy: %v", runErr)
		default:
		}
		t.Fatal(err)
	}

	// Startup must have brought the database to the current schema and
	// created the admin token table.
	conn, err := sql.Open("sqlite3", os.Getenv("JOBS_DB_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var version int
	if err := conn.QueryRow(`SELECT coalesce(max(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("schema version = %d, want 3 after run() startup", version)
	}
	var tokenCount int
	if err := conn.QueryRow(`SELECT count(*) FROM admin_tokens`).Scan(&tokenCount); err != nil {
		t.Fatalf("admin_tokens table should exist after startup: %v", err)
	}

	select {
	case runErr := <-done:
		t.Fatalf("run() should still be serving: %v", runErr)
	default:
	}
}

func TestMainRunsAndExitsNonZeroOnBadDatabasePath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
	// Run the compiled binary with a directory as the sqlite path: startup
	// validation must fail and terminate the process instead of serving.
	binary := filepath.Join(t.TempDir(), "admin-test-binary")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build test binary: %v\n%s", err, out)
	}
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("JOBS_DB_PATH=%s", t.TempDir()), // a directory is not a usable sqlite file
		"JOBS_ADMIN_ADDR=127.0.0.1:0",
	)
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errorsAs(err, &exitErr) {
		t.Fatalf("main should exit non-zero on unusable database, got err=%v", err)
	}
}

// errorsAs avoids importing "errors" only for one call site.
func errorsAs(err error, target **exec.ExitError) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		*target = exitErr
		return true
	}
	return false
}

func waitHealthy(ctx context.Context, addr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("healthz status = %d", res.StatusCode)
		} else if res != nil {
			res.Body.Close()
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("admin service never became healthy: %w", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
