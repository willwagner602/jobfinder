package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthIsLivenessOnlyAndUnauthenticated(t *testing.T) {
	store := newTestStore(t)
	server := NewServer(store.DB, "secret", nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	server.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	if body := strings.TrimSpace(res.Body.String()); body != `{"status":"ok"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestAdminRoutesRequireToken(t *testing.T) {
	store := newTestStore(t)
	server := NewServer(store.DB, "secret", nil)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	res := httptest.NewRecorder()
	server.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.Code)
	}
}

func TestProcessHistoryPageShowsCountsAndErrors(t *testing.T) {
	store := newTestStore(t)
	_, err := store.DB.Exec(`INSERT INTO job_history (service, started_at, status, items_processed, error_json) VALUES ('job-match', '2025-01-01T00:00:00Z', 'failure', 1, '{"error":"boom"}')`)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store.DB, "secret", nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/process-history", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	server.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, "failure: 1") || !strings.Contains(body, "boom") {
		t.Fatalf("history page missing status count or error detail: %s", body)
	}
}
