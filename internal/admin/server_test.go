package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

func TestIssuedTokenAuthorizesUntilRevokedAndBootstrapRemainsSeparate(t *testing.T) {
	store := newTestStore(t)
	server := NewServer(store.DB, "bootstrap", nil)
	handler := server.Routes()

	issueForm := url.Values{"subject": {"ops"}, "issued_to": {"ops@example.com"}, "ttl_hours": {"1"}}
	issueReq := httptest.NewRequest(http.MethodPost, "/admin/tokens/issue", strings.NewReader(issueForm.Encode()))
	issueReq.Header.Set("Authorization", "Bearer bootstrap")
	issueReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	issueRes := httptest.NewRecorder()
	handler.ServeHTTP(issueRes, issueReq)
	if issueRes.Code != http.StatusOK {
		t.Fatalf("issue status = %d body=%s", issueRes.Code, issueRes.Body.String())
	}
	var issued IssuedToken
	if err := json.NewDecoder(issueRes.Body).Decode(&issued); err != nil {
		t.Fatalf("decode issued token: %v", err)
	}
	if issued.Token == "" || issued.Token == "bootstrap" {
		t.Fatalf("issued token = %q", issued.Token)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	adminReq.Header.Set("Authorization", "Bearer "+issued.Token)
	adminRes := httptest.NewRecorder()
	handler.ServeHTTP(adminRes, adminReq)
	if adminRes.Code != http.StatusOK {
		t.Fatalf("issued token admin status = %d body=%s", adminRes.Code, adminRes.Body.String())
	}

	revokeForm := url.Values{"token_id": {strconv.FormatInt(issued.ID, 10)}}
	revokeReq := httptest.NewRequest(http.MethodPost, "/admin/tokens/revoke", strings.NewReader(revokeForm.Encode()))
	revokeReq.Header.Set("Authorization", "Bearer bootstrap")
	revokeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revokeRes := httptest.NewRecorder()
	handler.ServeHTTP(revokeRes, revokeReq)
	if revokeRes.Code != http.StatusSeeOther {
		t.Fatalf("revoke status = %d body=%s", revokeRes.Code, revokeRes.Body.String())
	}

	revokedReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	revokedReq.Header.Set("Authorization", "Bearer "+issued.Token)
	revokedRes := httptest.NewRecorder()
	handler.ServeHTTP(revokedRes, revokedReq)
	if revokedRes.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", revokedRes.Code)
	}

	bootstrapReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	bootstrapReq.Header.Set("Authorization", "Bearer bootstrap")
	bootstrapRes := httptest.NewRecorder()
	handler.ServeHTTP(bootstrapRes, bootstrapReq)
	if bootstrapRes.Code != http.StatusOK {
		t.Fatalf("bootstrap status after issued-token revoke = %d", bootstrapRes.Code)
	}
}
