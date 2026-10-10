package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/schema"
)

type captureNotifier struct{ codes map[string]string }

func (n *captureNotifier) SendCode(_ context.Context, email, code string) error {
	if n.codes == nil {
		n.codes = map[string]string{}
	}
	n.codes[email] = code
	return nil
}

type captureRunner struct {
	calls      int
	userID     int64
	start, end time.Time
}

func (r *captureRunner) RunPriorWeek(_ context.Context, userID int64, start, end time.Time) error {
	r.calls++
	r.userID = userID
	r.start = start
	r.end = end
	return nil
}

func TestProtectedPagesRequireAuth(t *testing.T) {
	s := newTestServer(t)
	for _, path := range []string{"/", "/onboarding", "/jobs"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		s.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
			t.Fatalf("%s got status %d location %q, want redirect to /login", path, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestSignupVerifyLoginAndOnboarding(t *testing.T) {
	notifier := &captureNotifier{}
	runner := &captureRunner{}
	s := newTestServer(t)
	s.Notifier = notifier
	s.MatchRunner = runner
	h := s.Routes()

	csrf, cookies := getCSRF(t, h, "/signup")
	postForm(t, h, "/signup", url.Values{"email": {"User@Example.COM"}, "password": {"correct horse battery staple"}, "csrf": {csrf}}, cookies)
	if notifier.codes["user@example.com"] == "" {
		t.Fatal("signup did not send verification code")
	}

	csrf, cookies = getCSRF(t, h, "/verify?email=user%40example.com")
	verifyRec := postForm(t, h, "/verify", url.Values{"email": {"user@example.com"}, "code": {notifier.codes["user@example.com"]}, "csrf": {csrf}}, cookies)
	if verifyRec.Code != http.StatusSeeOther || verifyRec.Header().Get("Location") != "/onboarding" {
		t.Fatalf("verify got %d location %q", verifyRec.Code, verifyRec.Header().Get("Location"))
	}
	sessionCookies := verifyRec.Result().Cookies()

	var status string
	var confirmed sql.NullString
	if err := s.DB.QueryRow(`SELECT status, email_confirmed_at FROM users WHERE email='user@example.com'`).Scan(&status, &confirmed); err != nil {
		t.Fatal(err)
	}
	if status != string(schema.UserStatusLive) || !confirmed.Valid {
		t.Fatalf("user not live/confirmed: %s %+v", status, confirmed)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range sessionCookies {
		req.AddCookie(c)
	}
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated home status %d", rec.Code)
	}

	csrf, cookies = getCSRFWithCookies(t, h, "/onboarding", sessionCookies)
	cookies = append(cookies, sessionCookies...)
	postForm(t, h, "/onboarding", url.Values{"resume_text": {"resume text"}, "experience_text": {"bullet one"}, "desired_titles": {"Engineer\nStaff Engineer"}, "search_preferences": {"remote only"}, "blocked_companies": {"BadCo"}, "csrf": {csrf}}, cookies)
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if runner.end.Sub(runner.start) != 7*24*time.Hour {
		t.Fatalf("match window = %s, want 7d", runner.end.Sub(runner.start))
	}
	var resume, titles string
	if err := s.DB.QueryRow(`SELECT resume_text, desired_titles_json FROM user_configs WHERE user_id = ?`, runner.userID).Scan(&resume, &titles); err != nil {
		t.Fatal(err)
	}
	if resume != "resume text" || !strings.Contains(titles, "Staff Engineer") {
		t.Fatalf("profile not saved: resume=%q titles=%q", resume, titles)
	}
}

func TestCodeIsConsumedOnce(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	uid := insertLiveUser(t, s.DB, "code@example.com")
	if err := s.issueCode(context.Background(), "code@example.com", "login", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	code := notifier.codes["code@example.com"]
	if code == "" {
		t.Fatal("no code captured")
	}
	if !s.verifyCode(context.Background(), uid, code, "login") {
		t.Fatal("first code use failed")
	}
	if s.verifyCode(context.Background(), uid, code, "login") {
		t.Fatal("replayed code succeeded")
	}
	var consumed int
	if err := s.DB.QueryRow(`SELECT count(*) FROM web_auth_codes WHERE user_id = ? AND consumed_at IS NOT NULL`, uid).Scan(&consumed); err != nil {
		t.Fatal(err)
	}
	if consumed != 1 {
		t.Fatalf("consumed rows = %d, want 1", consumed)
	}
}

func TestCodeConsumeUpdateIsAtomic(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	uid := insertLiveUser(t, s.DB, "atomic@example.com")
	if err := s.issueCode(context.Background(), "atomic@example.com", "login", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	var codeHash string
	if err := s.DB.QueryRow(`SELECT code_hash FROM web_auth_codes WHERE user_id = ?`, uid).Scan(&codeHash); err != nil {
		t.Fatal(err)
	}
	now := s.Now().Format(time.RFC3339Nano)
	res, err := s.DB.Exec(`UPDATE web_auth_codes SET consumed_at = ? WHERE auth_code_id = (SELECT auth_code_id FROM web_auth_codes WHERE user_id = ? AND purpose = ? AND code_hash = ? AND consumed_at IS NULL AND expires_at >= ? ORDER BY auth_code_id DESC LIMIT 1) AND consumed_at IS NULL`, now, uid, "login", codeHash, now)
	if err != nil {
		t.Fatal(err)
	}
	first, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	res, err = s.DB.Exec(`UPDATE web_auth_codes SET consumed_at = ? WHERE auth_code_id = (SELECT auth_code_id FROM web_auth_codes WHERE user_id = ? AND purpose = ? AND code_hash = ? AND consumed_at IS NULL AND expires_at >= ? ORDER BY auth_code_id DESC LIMIT 1) AND consumed_at IS NULL`, now, uid, "login", codeHash, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if first != 1 || second != 0 {
		t.Fatalf("atomic consume rows affected = %d then %d, want 1 then 0", first, second)
	}
}

func TestCodeReplayDoesNotRecordSecondUse(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	uid := insertLiveUser(t, s.DB, "history@example.com")
	if err := s.issueCode(context.Background(), "history@example.com", "login", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	code := notifier.codes["history@example.com"]
	if !s.verifyCode(context.Background(), uid, code, "login") {
		t.Fatal("first code use failed")
	}
	if s.verifyCode(context.Background(), uid, code, "login") {
		t.Fatal("replayed code succeeded")
	}
	var uses int
	if err := s.DB.QueryRow(`SELECT count(*) FROM user_history WHERE user_id = ? AND event_type = 'auth_code_used'`, uid).Scan(&uses); err != nil {
		t.Fatal(err)
	}
	if uses != 1 {
		t.Fatalf("use history rows = %d, want 1", uses)
	}
}

func TestSMTPNotifierRequiresConfiguration(t *testing.T) {
	if _, err := NewSMTPNotifier(SMTPConfig{}); err == nil {
		t.Fatal("missing SMTP config accepted")
	}
	n, err := NewSMTPNotifier(SMTPConfig{Addr: "smtp.example.com:587", Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Config.From != "jobs@willwagner.me" {
		t.Fatalf("default From = %q", n.Config.From)
	}
}

func TestJobPagesAreUserSpecific(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	uid1 := insertLiveUser(t, s.DB, "one@example.com")
	uid2 := insertLiveUser(t, s.DB, "two@example.com")
	matchID := insertMatch(t, s.DB, uid1)
	_ = uid2
	s.setSession(httptest.NewRecorder(), uid1)
	cookie := sessionCookieFor(t, s, uid1)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.AddCookie(cookie)
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Platform Engineer") {
		t.Fatalf("jobs list status=%d body=%s", rec.Code, rec.Body.String())
	}

	otherCookie := sessionCookieFor(t, s, uid2)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/jobs/"+strconvI64(matchID), nil)
	req.AddCookie(otherCookie)
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other user detail status=%d, want 404", rec.Code)
	}
	_ = ctx
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if err := ensureWebStorage(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	tpl, err := ParseTemplatesFS(fstest.MapFS{
		"base.html":       &fstest.MapFile{Data: []byte(`{{define "header"}}<html><body>{{if .Error}}{{.Error}}{{end}}{{if .Notice}}{{.Notice}}{{end}}{{end}}{{define "footer"}}</body></html>{{end}}`)},
		"signup.html":     &fstest.MapFile{Data: []byte(`{{template "header" .}}<form>{{.CSRF}}</form>{{template "footer" .}}`)},
		"verify.html":     &fstest.MapFile{Data: []byte(`{{template "header" .}}<form>{{.CSRF}}</form>{{template "footer" .}}`)},
		"login.html":      &fstest.MapFile{Data: []byte(`{{template "header" .}}<form>{{.CSRF}}</form>{{template "footer" .}}`)},
		"home.html":       &fstest.MapFile{Data: []byte(`{{template "header" .}}home {{.User.Email}}{{template "footer" .}}`)},
		"onboarding.html": &fstest.MapFile{Data: []byte(`{{template "header" .}}onboarding{{template "footer" .}}`)},
		"jobs.html":       &fstest.MapFile{Data: []byte(`{{template "header" .}}{{range .Jobs}}{{.Title}}{{end}}{{template "footer" .}}`)},
		"job.html":        &fstest.MapFile{Data: []byte(`{{template "header" .}}{{.Job.Title}}{{template "footer" .}}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Server{DB: conn, Notifier: &captureNotifier{}, MatchRunner: &captureRunner{}, Templates: tpl, SessionKey: []byte("0123456789abcdef0123456789abcdef"), Now: func() time.Time { return time.Date(2025, 1, 8, 12, 0, 0, 0, time.UTC) }}
}

func getCSRF(t *testing.T, h http.Handler, path string) (string, []*http.Cookie) {
	return getCSRFWithCookies(t, h, path, nil)
}

func getCSRFWithCookies(t *testing.T, h http.Handler, path string, in []*http.Cookie) (string, []*http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range in {
		req.AddCookie(c)
	}
	h.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName {
			return c.Value, rec.Result().Cookies()
		}
	}
	t.Fatal("no csrf cookie")
	return "", nil
}

func postForm(t *testing.T, h http.Handler, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	h.ServeHTTP(rec, req)
	return rec
}

func insertLiveUser(t *testing.T, conn *sql.DB, email string) int64 {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO users(email, email_confirmed_at, status) VALUES (?, '2025-01-01T00:00:00Z', ?)`, email, schema.UserStatusLive)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertMatch(t *testing.T, conn *sql.DB, userID int64) int64 {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO job_boards(board_url, company, status) VALUES ('https://example.com/jobs', 'ExampleCo', 'active')`)
	if err != nil {
		t.Fatal(err)
	}
	boardID, _ := res.LastInsertId()
	if _, err := db.UpsertListingVersion(context.Background(), conn, db.ListingVersionInput{CanonicalURL: "https://example.com/jobs/1", BoardID: boardID, Title: "Platform Engineer", Body: "Build useful systems", FoundAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var listingID int64
	if err := conn.QueryRow(`SELECT job_listing_id FROM job_listings`).Scan(&listingID); err != nil {
		t.Fatal(err)
	}
	res, err = conn.Exec(`INSERT INTO job_matches(user_id, job_listing_id, job_listing_version, rating, decision, matcher_name) VALUES (?, ?, 1, 5, 'match', 'test')`, userID, listingID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func sessionCookieFor(t *testing.T, s *Server, uid int64) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	s.setSession(rec, uid)
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func strconvI64(v int64) string { return strconv.FormatInt(v, 10) }
