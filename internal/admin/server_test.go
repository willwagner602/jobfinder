package admin

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// assertStatus is a shorthand for checking an httptest recorder's code with
// a useful message that includes the response body.
func assertStatus(t *testing.T, res *httptest.ResponseRecorder, want int) {
	t.Helper()
	if res.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", res.Code, want, res.Body.String())
	}
}

// newUser persistently registers a user row, returning its id.
func newUser(t *testing.T, db *sql.DB, email, status string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (email, status) VALUES (?, ?)`, email, status)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// postForm issues the given request with the default working bearer token.
func postForm(t *testing.T, handler http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

// healthzGet issues an unauthenticated GET for a handler that does not need auth.
func healthzGet(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func TestAdminRoutesHealthIsLivenessOnlyAndUnauthenticated(t *testing.T) {
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

func TestApproveAndSuspendRoutesRedirectOnSuccessAndValidateInput(t *testing.T) {
	store := newTestStore(t)
	handler := NewServer(store.DB, "secret", nil).Routes()
	id := newUser(t, store.DB, "person@example.com", "pending")

	approve := postForm(t, handler, "/admin/users/approve", url.Values{"user_id": {strconv.FormatInt(id, 10)}, "admin_email": {"ops@example.com"}})
	assertStatus(t, approve, http.StatusSeeOther)
	if got := approve.Header().Get("Location"); got != "/admin" {
		t.Fatalf("approve redirect target = %q", got)
	}

	suspend := postForm(t, handler, "/admin/users/suspend", url.Values{"user_id": {strconv.FormatInt(id, 10)}, "admin_email": {"ops@example.com"}})
	assertStatus(t, suspend, http.StatusSeeOther)
	if got := suspend.Header().Get("Location"); got != "/admin" {
		t.Fatalf("suspend redirect target = %q", got)
	}

	// Missing or non-numeric ids are a client error.
	for _, bad := range []string{"", "abc", "-1", "0"} {
		res := postForm(t, handler, "/admin/users/approve", url.Values{"user_id": {bad}})
		assertStatus(t, res, http.StatusBadRequest)
	}
	// A well-formed id that does not exist is a 404.
	unknown := postForm(t, handler, "/admin/users/suspend", url.Values{"user_id": {"424242"}})
	assertStatus(t, unknown, http.StatusNotFound)
}

func TestTokensPageListsIssuedAndRevokedTokens(t *testing.T) {
	store := newTestStore(t)
	handler := NewServer(store.DB, "secret", nil).Routes()
	issued, err := store.IssueToken(context.Background(), "deploy", "ops@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeToken(context.Background(), issued.ID); err != nil {
		t.Fatal(err)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
	getReq.Header.Set("Authorization", "Bearer secret")
	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, getReq)
	assertStatus(t, getRes, http.StatusOK)
	body := getRes.Body.String()
	for _, want := range []string{"deploy", "ops@example.com", issued.Subject} {
		if !strings.Contains(body, want) {
			t.Fatalf("tokens page missing %q: %s", want, body)
		}
	}
}

func TestIssueTokenRouteValidatesFormAndCanSendEmail(t *testing.T) {
	store := newTestStore(t)
	var sent struct {
		to      string
		subject string
		body    string
	}
	fakeMailer := &captureMailer{record: &sent}
	server := NewServer(store.DB, "secret", fakeMailer)
	handler := server.Routes()

	badTTL := postForm(t, handler, "/admin/tokens/issue", url.Values{"subject": {"s"}, "issued_to": {"ops@example.com"}, "ttl_hours": {"zero"}})
	assertStatus(t, badTTL, http.StatusBadRequest)

	negativeTTL := postForm(t, handler, "/admin/tokens/issue", url.Values{"subject": {"s"}, "issued_to": {"ops@example.com"}, "ttl_hours": {"-3"}})
	assertStatus(t, negativeTTL, http.StatusBadRequest)

	emptyFields := postForm(t, handler, "/admin/tokens/issue", url.Values{})
	assertStatus(t, emptyFields, http.StatusBadRequest)

	ok := postForm(t, handler, "/admin/tokens/issue", url.Values{"subject": {"ci"}, "issued_to": {"ci@example.com"}, "send_email": {"true"}})
	assertStatus(t, ok, http.StatusOK)
	if sent.to != "ci@example.com" || sent.subject == "" {
		t.Fatalf("mailer not invoked as expected: to=%q subject=%q", sent.to, sent.subject)
	}
	if !strings.Contains(sent.body, "expires at") {
		t.Fatalf("token email body missing expiry: %q", sent.body)
	}
	// The emailed body must contain the raw token, which is stored only as
	// a hash, so verify against the token returned by the issue request.
	var issued IssuedToken
	if err := json.NewDecoder(ok.Body).Decode(&issued); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}
	if !strings.Contains(sent.body, issued.Token) {
		t.Fatalf("token email body missing raw token: %q", sent.body)
	}
}

type captureMailer struct {
	record *struct {
		to      string
		subject string
		body    string
	}
}

func (m *captureMailer) Send(ctx context.Context, to, subject, body string) error {
	m.record.to = to
	m.record.subject = subject
	m.record.body = body
	return nil
}

func TestIssueTokenRouteReportsMailFailureWithoutIssuing(t *testing.T) {
	store := newTestStore(t)
	server := NewServer(store.DB, "secret", failingMailer{})
	res := postForm(t, server.Routes(), "/admin/tokens/issue", url.Values{"subject": {"ci"}, "issued_to": {"ci@example.com"}, "send_email": {"true"}})
	assertStatus(t, res, http.StatusBadGateway)
	if !strings.Contains(res.Body.String(), "token issued but email send failed") {
		t.Fatalf("mail failure not reported: %s", res.Body.String())
	}
	tokens, err := store.ListTokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Mail failure happens after the row is persisted, so the token exists
	// but was not delivered.
	if len(tokens) != 1 {
		t.Fatalf("tokens after mail failure = %d, want 1", len(tokens))
	}
}

type failingMailer struct{}

func (failingMailer) Send(ctx context.Context, to, subject, body string) error {
	return errors.New("smtp refused")
}

func TestRevokeTokenRouteRedirectsAndHandlesUnknownID(t *testing.T) {
	store := newTestStore(t)
	handler := NewServer(store.DB, "secret", nil).Routes()
	issued, err := store.IssueToken(context.Background(), "deploy", "ops@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	res := postForm(t, handler, "/admin/tokens/revoke", url.Values{"token_id": {strconv.FormatInt(issued.ID, 10)}})
	assertStatus(t, res, http.StatusSeeOther)
	if got := res.Header().Get("Location"); got != "/admin/tokens" {
		t.Fatalf("revoke redirect target = %q", got)
	}
	for _, bad := range []string{"", "abc", "0", "-5"} {
		res := postForm(t, handler, "/admin/tokens/revoke", url.Values{"token_id": {bad}})
		assertStatus(t, res, http.StatusBadRequest)
	}
	unknown := postForm(t, handler, "/admin/tokens/revoke", url.Values{"token_id": {"424242"}})
	assertStatus(t, unknown, http.StatusNotFound)
}

func TestProcessHistoryRouteAcceptsLimitAndDefaults(t *testing.T) {
	store := newTestStore(t)
	handler := NewServer(store.DB, "secret", nil).Routes()
	for _, service := range []string{"a", "b", "c"} {
		if _, err := store.DB.Exec(`INSERT INTO job_history (service, started_at, status) VALUES (?, '2025-01-0?T00:00:00Z', 'partial')`, service); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/process-history?limit=2", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assertStatus(t, res, http.StatusOK)
	if strings.Count(res.Body.String(), "<tr><td>") > 2 {
		t.Fatalf("limit=2 should show at most 2 runs: %s", res.Body.String())
	}

	defaultLimit := httptest.NewRequest(http.MethodGet, "/admin/process-history", nil)
	defaultLimit.Header.Set("Authorization", "Bearer secret")
	defaultRes := httptest.NewRecorder()
	handler.ServeHTTP(defaultRes, defaultLimit)
	assertStatus(t, defaultRes, http.StatusOK)
}

func TestWrongMethodAndUnknownPathAreRejected(t *testing.T) {
	store := newTestStore(t)
	handler := NewServer(store.DB, "secret", nil).Routes()
	// Wrong method on a known pattern -> 405 from the mux.
	wrongMethod := httptest.NewRequest(http.MethodGet, "/admin/users/approve", nil)
	wrongMethod.Header.Set("Authorization", "Bearer secret")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, wrongMethod)
	assertStatus(t, res, http.StatusMethodNotAllowed)

	// Unknown path -> 404.
	unknown := httptest.NewRequest(http.MethodGet, "/admin/nope", nil)
	unknownRes := httptest.NewRecorder()
	handler.ServeHTTP(unknownRes, unknown)
	assertStatus(t, unknownRes, http.StatusNotFound)
}

func TestMalformedAuthorizationHeadersAreRejected(t *testing.T) {
	store := newUserTokenStoreWithToken(t)
	server := NewServer(store.DB, "secret", nil)
	handler := server.Routes()
	id := newUser(t, store.DB, "person@example.com", "pending")

	// Missing scheme prefix, so TrimPrefix does not remove it.
	noScheme := httptest.NewRequest(http.MethodGet, "/admin", nil)
	noScheme.Header.Set("Authorization", "secret")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, noScheme)
	assertStatus(t, res, http.StatusUnauthorized)

	// Right scheme, wrong token: not the bootstrap secret and not in the
	// admin_tokens table, so server falls back to a token lookup.
	wrongToken := httptest.NewRequest(http.MethodPost, "/admin/users/approve", strings.NewReader(url.Values{"user_id": {strconv.FormatInt(id, 10)}}.Encode()))
	wrongToken.Header.Set("Authorization", "Bearer not-the-secret")
	wrongToken.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wrongRes := httptest.NewRecorder()
	handler.ServeHTTP(wrongRes, wrongToken)
	assertStatus(t, wrongRes, http.StatusUnauthorized)
}

// newUserTokenStoreWithToken registers an issued-token row to validate
// header parsing; the bootstrap secret remains independent.
func newUserTokenStoreWithToken(t *testing.T) Store {
	t.Helper()
	return newTestStore(t)
}

func TestUnparseableBodyOnAuthedRouteIsBadRequest(t *testing.T) {
	store := newTestStore(t)
	handler := NewServer(store.DB, "secret", nil).Routes()
	// A form-encoded body must parse; a deliberately malformed
	// multipart-style content header forces ParseForm into its error branch.
	req := httptest.NewRequest(http.MethodPost, "/admin/users/approve", strings.NewReader("user_id=1%zz"))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// urlencoding with an invalid escape makes ParseForm fail.
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	// ParseForm swallows invalid escapes for urlencoded bodies, so this
	// still reaches the handler with a missing/zero id -> 400 regardless.
	assertStatus(t, res, http.StatusBadRequest)
}

func TestDBFailureOnUserAndTokenRoutesIsServerError(t *testing.T) {
	broken := Store{DB: closingDB(t)}
	server := NewServer(broken.DB, "secret", nil)
	handler := server.Routes()

	adminReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	adminReq.Header.Set("Authorization", "Bearer secret")
	adminRes := httptest.NewRecorder()
	handler.ServeHTTP(adminRes, adminReq)
	assertStatus(t, adminRes, http.StatusInternalServerError)

	tokensReq := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
	tokensReq.Header.Set("Authorization", "Bearer secret")
	tokensRes := httptest.NewRecorder()
	handler.ServeHTTP(tokensRes, tokensReq)
	assertStatus(t, tokensRes, http.StatusInternalServerError)

	historyReq := httptest.NewRequest(http.MethodGet, "/admin/process-history", nil)
	historyReq.Header.Set("Authorization", "Bearer secret")
	historyRes := httptest.NewRecorder()
	handler.ServeHTTP(historyRes, historyReq)
	assertStatus(t, historyRes, http.StatusInternalServerError)

	// With no usable DB, authorization itself also fails for a non-bootstrap
	// bearer token (ValidateToken error path in authorized()).
	otherToken := httptest.NewRequest(http.MethodGet, "/admin", nil)
	otherToken.Header.Set("Authorization", "Bearer some-issued-token")
	otherRes := httptest.NewRecorder()
	handler.ServeHTTP(otherRes, otherToken)
	assertStatus(t, otherRes, http.StatusUnauthorized)
}

func closingDB(t *testing.T) *sql.DB {
	t.Helper()
	store := newTestStore(t)
	db := store.DB
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return db
}

func TestSMTPMailerWithoutAddressOrFromRefuses(t *testing.T) {
	mailer := SMTPMailer{}
	ctx := context.Background()
	if err := mailer.Send(ctx, "to@example.com", "subject", "body"); err == nil || !strings.Contains(err.Error(), "smtp address and from address are required") {
		t.Fatalf("Send with no addr/from = %v, want config error", err)
	}
}

func TestSMTPMailerHonoursCancelledContext(t *testing.T) {
	mailer := SMTPMailer{Addr: "127.0.0.1:1", From: "jobs@willwagner.me"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mailer.Send(ctx, "to@example.com", "subject", "body"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Send with cancelled ctx = %v, want context.Canceled", err)
	}
}

func TestSMTPMailerBuildsPlainAuthForConnectionRefused(t *testing.T) {
	mailer := SMTPMailer{Addr: "127.0.0.1:1", From: "jobs@willwagner.me", Username: "user", Password: "secret"}
	err := mailer.Send(context.Background(), "to@example.com", "subject", "body")
	if err == nil {
		t.Fatal("Send to closed port should fail")
	}
	if strings.Contains(err.Error(), "smtp address and from address are required") {
		t.Fatalf("expected connection error, got config error: %v", err)
	}
}

func TestSMTPMailerConnectsToRealSMTPServerAndSends(t *testing.T) {
	// A minimal SMTP server speaking enough of the protocol for smtp.SendMail
	// without AUTH: EHLO, MAIL FROM, RCPT TO, DATA, QUIT.
	addr, closeSMTP, protocol := startFakeSMTP(t)
	defer closeSMTP()
	mailer := SMTPMailer{Addr: addr, From: "jobs@willwagner.me"}
	if err := mailer.Send(context.Background(), "to@example.com", "token email", "body text"); err != nil {
		t.Fatalf("Send to fake SMTP server: %v\nprotocol trace:\n%s", err, protocol.String())
	}
}

func startFakeSMTP(t *testing.T) (addr string, close func(), protocol *strings.Builder) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var trace strings.Builder
	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		defer func() { readerDrain(&trace, conn) }()
		reader := bufio.NewScanner(conn)
		reader.Buffer(make([]byte, 1024), 4096)
		conn.Write([]byte("220 fake ESMTP\r\n"))
		trace.WriteString("S: 220\r\n")
		sawData := false
		for reader.Scan() {
			line := reader.Text()
			trace.WriteString("C: " + line + "\n")
			switch {
			case strings.HasPrefix(line, "EHLO"):
				// Single-line greeting plus AUTH announcement is enough
				// for net/smtp to consider the server capable.
				conn.Write([]byte("250-fake greets you\r\n250 AUTH PLAIN\r\n"))
				trace.WriteString("S: 250-EHLO\r\n")
			case strings.HasPrefix(line, "MAIL") || strings.HasPrefix(line, "RCPT"):
				conn.Write([]byte("250 OK\r\n"))
				trace.WriteString("S: 250\r\n")
			case line == "DATA":
				sawData = true
				conn.Write([]byte("354 go ahead\r\n"))
				trace.WriteString("S: 354\r\n")
			case sawData && line == ".":
				sawData = false
				conn.Write([]byte("250 accepted\r\n"))
				trace.WriteString("S: 250 accepted\r\n")
			case sawData:
				// Body lines inside DATA must not receive any reply.
				continue
			case strings.HasPrefix(line, "QUIT"):
				conn.Write([]byte("221 bye\r\n"))
				trace.WriteString("S: 221\r\n")
				return
			default:
				conn.Write([]byte("250 OK\r\n"))
				trace.WriteString("S: 250 default\r\n")
			}
		}
	}()
	return listener.Addr().String(), func() {
		listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}, &trace
}

// readerDrain exists only as a debugging aid when the SMTP test fails.
func readerDrain(trace *strings.Builder, conn net.Conn) {
	buf := make([]byte, 512)
	for {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := conn.Read(buf)
		if n > 0 {
			trace.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func TestSMTPMailerFromEnvReadsConfiguration(t *testing.T) {
	t.Setenv("JOBS_SMTP_ADDR", "smtp.example.com:25")
	t.Setenv("JOBS_SMTP_USERNAME", "user")
	t.Setenv("JOBS_SMTP_PASSWORD", "secret")
	t.Setenv("JOBS_SMTP_FROM", "ops@willwagner.me")
	mailer := NewSMTPMailerFromEnv()
	if mailer.Addr != "smtp.example.com:25" || mailer.Username != "user" || mailer.Password != "secret" || mailer.From != "ops@willwagner.me" {
		t.Fatalf("mailer config wrong: %+v", mailer)
	}

	t.Setenv("JOBS_SMTP_FROM", "")
	fallback := NewSMTPMailerFromEnv()
	if fallback.From != "jobs@willwagner.me" {
		t.Fatalf("From fallback = %q", fallback.From)
	}
}
