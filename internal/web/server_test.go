package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"jobs.willwagner.me/internal/admin"
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

type failingNotifier struct {
	codes map[string]string
	err   error
}

func (n *failingNotifier) SendCode(_ context.Context, email, code string) error {
	if n.codes == nil {
		n.codes = map[string]string{}
	}
	n.codes[email] = code
	if n.err != nil {
		return n.err
	}
	return errors.New("send failed")
}

type commitAwareNotifier struct {
	t       *testing.T
	db      *sql.DB
	userID  int64
	codes   map[string]string
	checked bool
}

func (n *commitAwareNotifier) SendCode(_ context.Context, email, code string) error {
	n.checked = true
	var codes int
	if err := n.db.QueryRow(`SELECT count(*) FROM web_auth_codes WHERE user_id = ?`, n.userID).Scan(&codes); err != nil {
		n.t.Fatalf("query auth codes during SendCode: %v", err)
	}
	if codes != 1 {
		n.t.Fatalf("auth codes visible during SendCode = %d, want 1", codes)
	}
	if _, err := n.db.Exec(`UPDATE users SET updated_at = updated_at WHERE user_id = ?`, n.userID); err != nil {
		n.t.Fatalf("write during SendCode failed; signup transaction may still be open: %v", err)
	}
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

func TestSPARoutesServeIndex(t *testing.T) {
	s := newTestServer(t)
	h := s.Routes()
	for _, path := range []string{"/", "/signup", "/verify", "/login", "/onboarding", "/jobs", "/jobs/123"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Fatalf("%s content type = %q, want SPA HTML", path, got)
		}
		if !strings.Contains(rec.Body.String(), "vue-spa") || strings.Contains(rec.Body.String(), "home ") {
			t.Fatalf("%s did not serve SPA index: %s", path, rec.Body.String())
		}
	}
}

func TestLegacyUserRoutesOnlyWhenSPAIndexMissing(t *testing.T) {
	s := newTestServer(t)
	s.Assets = http.FS(fstest.MapFS{})
	h := s.Routes()
	uid := insertLiveUser(t, s.DB, "legacy@example.com")
	matchID := insertMatch(t, s.DB, uid)
	session := []*http.Cookie{sessionCookieFor(t, s, uid)}

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/signup", "<form>"},
		{"/verify", "<form>"},
		{"/login", "<form>"},
		{"/", "legacy@example.com"},
		{"/onboarding", "onboarding"},
		{"/jobs", "Platform Engineer"},
		{"/jobs/" + strconvI64(matchID), "Platform Engineer"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		for _, c := range session {
			req.AddCookie(c)
		}
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s status=%d body=%s want %q", tc.path, rec.Code, rec.Body.String(), tc.want)
		}
	}
}

func TestSPARouteFallbackDoesNotShadowReservedRoutes(t *testing.T) {
	s := newTestServer(t)
	h := s.Routes()

	apiRec := getJSON(t, h, "/api/profile", nil)
	if apiRec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/profile status = %d, want 401", apiRec.Code)
	}

	healthRec := httptest.NewRecorder()
	h.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthRec.Code != http.StatusNoContent {
		t.Fatalf("/healthz status = %d, want 204", healthRec.Code)
	}

	assetRec := httptest.NewRecorder()
	h.ServeHTTP(assetRec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if assetRec.Code != http.StatusOK || !strings.Contains(assetRec.Body.String(), "built asset") {
		t.Fatalf("/assets/app.js status=%d body=%s", assetRec.Code, assetRec.Body.String())
	}

	for _, path := range []string{"/api/unknown", "/admin", "/internal", "/static/missing.css", "/does-not-exist"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "vue-spa") {
			t.Fatalf("%s unexpectedly served SPA index", path)
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

	csrf, cookies = getAPICSRF(t, h, sessionCookies)
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

func TestJSONAPISignupVerifyLoginProfileAndJobs(t *testing.T) {
	notifier := &captureNotifier{}
	runner := &captureRunner{}
	s := newTestServer(t)
	s.Notifier = notifier
	s.MatchRunner = runner
	h := s.Routes()

	csrf, cookies := getAPICSRF(t, h, nil)
	rec := postJSON(t, h, "/api/signup", map[string]string{"email": "API@Example.COM", "password": "correct horse battery staple"}, csrf, cookies)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("api signup status=%d body=%s", rec.Code, rec.Body.String())
	}
	if notifier.codes["api@example.com"] == "" {
		t.Fatal("api signup did not send verification code")
	}

	csrf, _ = getAPICSRF(t, h, cookies)
	verifyRec := postJSON(t, h, "/api/verify", map[string]string{"email": "api@example.com", "code": notifier.codes["api@example.com"]}, csrf, cookies)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("api verify status=%d body=%s", verifyRec.Code, verifyRec.Body.String())
	}
	cookies = append(cookies, verifyRec.Result().Cookies()...)

	csrf, _ = getAPICSRF(t, h, cookies)
	profileRec := putJSON(t, h, "/api/profile", map[string]string{"resumeText": "resume text", "experienceText": "bullet one", "desiredTitles": "Engineer\nStaff Engineer", "searchPreferences": "remote only", "blockedCompanies": "BadCo"}, csrf, cookies)
	if profileRec.Code != http.StatusOK {
		t.Fatalf("api profile update status=%d body=%s", profileRec.Code, profileRec.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	matchID := insertMatch(t, s.DB, runner.userID)

	jobsRec := getJSON(t, h, "/api/jobs", cookies)
	if jobsRec.Code != http.StatusOK || !strings.Contains(jobsRec.Body.String(), "Platform Engineer") {
		t.Fatalf("api jobs status=%d body=%s", jobsRec.Code, jobsRec.Body.String())
	}
	detailRec := getJSON(t, h, "/api/jobs/"+strconvI64(matchID), cookies)
	if detailRec.Code != http.StatusOK || !strings.Contains(detailRec.Body.String(), "Platform Engineer") {
		t.Fatalf("api job detail status=%d body=%s", detailRec.Code, detailRec.Body.String())
	}
}

func TestJSONAPIAuthCSRFFailuresAndClientIdentity(t *testing.T) {
	s := newTestServer(t)
	h := s.Routes()
	uid1 := insertLiveUser(t, s.DB, "owner@example.com")
	uid2 := insertLiveUser(t, s.DB, "other@example.com")
	matchID := insertMatch(t, s.DB, uid1)
	ownerSession := []*http.Cookie{sessionCookieFor(t, s, uid1)}
	otherSession := []*http.Cookie{sessionCookieFor(t, s, uid2)}

	unauth := getJSON(t, h, "/api/profile", nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth profile status=%d body=%s", unauth.Code, unauth.Body.String())
	}

	csrf, cookies := getAPICSRF(t, h, ownerSession)
	badCSRF := putJSON(t, h, "/api/profile", map[string]string{"resumeText": "resume"}, "wrong-token", append(cookies, ownerSession...))
	if badCSRF.Code != http.StatusForbidden {
		t.Fatalf("bad csrf profile status=%d body=%s", badCSRF.Code, badCSRF.Body.String())
	}

	_ = csrf
	otherDetail := getJSON(t, h, "/api/jobs/"+strconvI64(matchID), otherSession)
	if otherDetail.Code != http.StatusNotFound {
		t.Fatalf("other user detail status=%d body=%s", otherDetail.Code, otherDetail.Body.String())
	}
}

func TestSignupDisabledBlocksSignup(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	if _, err := s.DB.Exec(`UPDATE site_config SET signups_disabled = 1 WHERE config_id = 1`); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()

	csrf, cookies := getCSRF(t, h, "/signup")
	rec := postForm(t, h, "/signup", url.Values{"email": {"closed@example.com"}, "password": {"correct horse battery staple"}, "csrf": {csrf}}, cookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("signup status = %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Signups are currently disabled") {
		t.Fatalf("signup body missing disabled message: %s", rec.Body.String())
	}
	var users int
	if err := s.DB.QueryRow(`SELECT count(*) FROM users WHERE email = 'closed@example.com'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 0 {
		t.Fatalf("users created = %d, want 0", users)
	}
	if notifier.codes["closed@example.com"] != "" {
		t.Fatal("signup sent verification code while disabled")
	}
}

func TestSignupRefusesTombstonedEmail(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	if _, err := s.DB.Exec(`INSERT INTO users(email, status) VALUES (?, ?)`, "gone@example.com", schema.UserStatusTombstoned); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()

	csrf, cookies := getCSRF(t, h, "/signup")
	rec := postForm(t, h, "/signup", url.Values{"email": {"gone@example.com"}, "password": {"correct horse battery staple"}, "csrf": {csrf}}, cookies)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("signup status = %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Email is not available for signup") {
		t.Fatalf("signup body missing generic refusal: %s", rec.Body.String())
	}
	var users int
	if err := s.DB.QueryRow(`SELECT count(*) FROM users WHERE email = 'gone@example.com'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("users with tombstoned email = %d, want 1", users)
	}
	if notifier.codes["gone@example.com"] != "" {
		t.Fatal("signup sent verification code for tombstoned email")
	}
}

func TestSignupStillHandlesSuspendedEmailWithExistingUpsert(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	if _, err := s.DB.Exec(`INSERT INTO users(email, status) VALUES (?, ?)`, "suspended@example.com", schema.UserStatusSuspended); err != nil {
		t.Fatal(err)
	}
	h := s.Routes()

	csrf, cookies := getCSRF(t, h, "/signup")
	rec := postForm(t, h, "/signup", url.Values{"email": {"suspended@example.com"}, "password": {"correct horse battery staple"}, "csrf": {csrf}}, cookies)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/verify?email=suspended%40example.com" {
		t.Fatalf("signup status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if notifier.codes["suspended@example.com"] == "" {
		t.Fatal("signup did not send verification code for suspended email")
	}
	var users int
	if err := s.DB.QueryRow(`SELECT count(*) FROM users WHERE email = 'suspended@example.com'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("users with suspended email = %d, want 1", users)
	}
}

func TestSignupTransactionRechecksTombstoneAfterEarlierLiveObservation(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	uid := insertLiveUser(t, s.DB, "race@example.com")

	var earlierStatus string
	if err := s.DB.QueryRow(`SELECT status FROM users WHERE email = 'race@example.com'`).Scan(&earlierStatus); err != nil {
		t.Fatal(err)
	}
	if earlierStatus != string(schema.UserStatusLive) {
		t.Fatalf("earlier status = %q, want live", earlierStatus)
	}
	if _, err := s.DB.Exec(`UPDATE users SET status = ? WHERE user_id = ?`, schema.UserStatusTombstoned, uid); err != nil {
		t.Fatal(err)
	}

	err := s.createSignupAndIssueCode(context.Background(), "race@example.com", sql.NullString{}, "127.0.0.1", nil)
	if !errors.Is(err, errEmailNotAvailable) {
		t.Fatalf("createSignupAndIssueCode error = %v, want errEmailNotAvailable", err)
	}
	var users int
	if err := s.DB.QueryRow(`SELECT count(*) FROM users WHERE email = 'race@example.com'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("users with raced email = %d, want 1", users)
	}
	if notifier.codes["race@example.com"] != "" {
		t.Fatal("signup sent verification code after tombstone")
	}
}

func TestSignupImmediateTransactionBlocksConcurrentTombstone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "signup-race.db")
	conn, err := db.Open(dbPath)
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
	if _, err := conn.Exec(`PRAGMA busy_timeout = 1`); err != nil {
		t.Fatal(err)
	}
	adminConn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { adminConn.Close() })
	if _, err := adminConn.Exec(`PRAGMA busy_timeout = 1`); err != nil {
		t.Fatal(err)
	}
	notifier := &captureNotifier{}
	s := &Server{DB: conn, Notifier: notifier, MatchRunner: &captureRunner{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), Now: func() time.Time { return time.Date(2025, 1, 8, 12, 0, 0, 0, time.UTC) }}
	uid := insertLiveUser(t, conn, "locked@example.com")

	var tombstoneErr error
	err = s.createSignupAndIssueCode(context.Background(), "locked@example.com", sql.NullString{}, "127.0.0.1", func(ctx context.Context) error {
		tombstoneErr = admin.Store{DB: adminConn}.TombstoneUser(ctx, uid, "ops@example.com")
		return nil
	})
	if err != nil {
		t.Fatalf("createSignupAndIssueCode: %v", err)
	}
	if tombstoneErr == nil {
		t.Fatal("concurrent tombstone succeeded while signup BEGIN IMMEDIATE transaction was open")
	}
	var status string
	if err := conn.QueryRow(`SELECT status FROM users WHERE user_id = ?`, uid).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(schema.UserStatusLive) {
		t.Fatalf("status after blocked tombstone = %q, want live", status)
	}
	if notifier.codes["locked@example.com"] == "" {
		t.Fatal("signup did not send verification code")
	}
}

func TestSignupSendsCodeAfterCommit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "signup-send-after-commit.db")
	conn, err := db.Open(dbPath)
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
	uid := insertLiveUser(t, conn, "after-commit@example.com")
	notifier := &commitAwareNotifier{t: t, db: conn, userID: uid}
	s := &Server{DB: conn, Notifier: notifier, MatchRunner: &captureRunner{}, SessionKey: []byte("0123456789abcdef0123456789abcdef"), Now: func() time.Time { return time.Date(2025, 1, 8, 12, 0, 0, 0, time.UTC) }}

	if err := s.createSignupAndIssueCode(context.Background(), "after-commit@example.com", sql.NullString{}, "127.0.0.1", nil); err != nil {
		t.Fatalf("createSignupAndIssueCode: %v", err)
	}
	if !notifier.checked {
		t.Fatal("notifier was not called")
	}
	if notifier.codes["after-commit@example.com"] == "" {
		t.Fatal("notifier did not capture code")
	}
}

func TestSignupSendFailureAfterCommitKeepsRows(t *testing.T) {
	notifier := &failingNotifier{err: errors.New("smtp down")}
	s := newTestServer(t)
	s.Notifier = notifier

	err := s.createSignupAndIssueCode(context.Background(), "durable@example.com", sql.NullString{}, "127.0.0.1", nil)
	if !errors.Is(err, errCodeDelivery) {
		t.Fatalf("createSignupAndIssueCode error = %v, want errCodeDelivery", err)
	}
	if notifier.codes["durable@example.com"] == "" {
		t.Fatal("notifier was not called")
	}
	var userID int64
	if err := s.DB.QueryRow(`SELECT user_id FROM users WHERE email = ? AND status = ?`, "durable@example.com", schema.UserStatusPending).Scan(&userID); err != nil {
		t.Fatalf("committed user missing after send failure: %v", err)
	}
	var codes, history int
	if err := s.DB.QueryRow(`SELECT count(*) FROM web_auth_codes WHERE user_id = ?`, userID).Scan(&codes); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM user_history WHERE user_id = ? AND event_type = 'auth_code_issued'`, userID).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if codes != 1 || history != 1 {
		t.Fatalf("durable rows after send failure: codes=%d history=%d, want 1/1", codes, history)
	}
}

func TestSignupDoesNotSendCodeWhenTransactionFailsBeforeCommit(t *testing.T) {
	notifier := &captureNotifier{}
	s := newTestServer(t)
	s.Notifier = notifier
	if _, err := s.DB.Exec(`UPDATE site_config SET signups_disabled = 1 WHERE config_id = 1`); err != nil {
		t.Fatal(err)
	}

	err := s.createSignupAndIssueCode(context.Background(), "nosend@example.com", sql.NullString{}, "127.0.0.1", nil)
	if !errors.Is(err, errSignupsDisabled) {
		t.Fatalf("createSignupAndIssueCode error = %v, want errSignupsDisabled", err)
	}
	if notifier.codes["nosend@example.com"] != "" {
		t.Fatal("notifier sent code on pre-commit refusal")
	}
	var users int
	if err := s.DB.QueryRow(`SELECT count(*) FROM users WHERE email = 'nosend@example.com'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 0 {
		t.Fatalf("users created = %d, want 0", users)
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

func TestJobAPIRoutesAreUserSpecific(t *testing.T) {
	s := newTestServer(t)
	uid1 := insertLiveUser(t, s.DB, "one@example.com")
	uid2 := insertLiveUser(t, s.DB, "two@example.com")
	matchID := insertMatch(t, s.DB, uid1)
	ownerCookie := sessionCookieFor(t, s, uid1)

	rec := getJSON(t, s.Routes(), "/api/jobs", []*http.Cookie{ownerCookie})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Platform Engineer") {
		t.Fatalf("jobs list status=%d body=%s", rec.Code, rec.Body.String())
	}

	otherCookie := sessionCookieFor(t, s, uid2)
	rec = getJSON(t, s.Routes(), "/api/jobs/"+strconvI64(matchID), []*http.Cookie{otherCookie})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other user detail status=%d, want 404", rec.Code)
	}
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
	return &Server{DB: conn, Notifier: &captureNotifier{}, MatchRunner: &captureRunner{}, Templates: tpl, Assets: fixtureAssets(), SessionKey: []byte("0123456789abcdef0123456789abcdef"), Now: func() time.Time { return time.Date(2025, 1, 8, 12, 0, 0, 0, time.UTC) }}
}

func fixtureAssets() http.FileSystem {
	return http.FS(fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(`<!doctype html><div id="app">vue-spa</div><script type="module" src="/assets/app.js"></script>`)},
		"assets/app.js": &fstest.MapFile{Data: []byte(`console.log("built asset")`)},
	})
}

func getAPICSRF(t *testing.T, h http.Handler, in []*http.Cookie) (string, []*http.Cookie) {
	t.Helper()
	rec := getJSON(t, h, "/api/session", in)
	if rec.Code != http.StatusOK {
		t.Fatalf("api session status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CSRFToken == "" {
		t.Fatal("empty api csrf token")
	}
	return body.CSRFToken, rec.Result().Cookies()
}

func getJSON(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	h.ServeHTTP(rec, req)
	return rec
}

func postJSON(t *testing.T, h http.Handler, path string, body any, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return requestJSON(t, h, http.MethodPost, path, body, csrf, cookies)
}

func putJSON(t *testing.T, h http.Handler, path string, body any, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return requestJSON(t, h, http.MethodPut, path, body, csrf, cookies)
}

func requestJSON(t *testing.T, h http.Handler, method, path string, body any, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	h.ServeHTTP(rec, req)
	return rec
}

func getCSRF(t *testing.T, h http.Handler, path string) (string, []*http.Cookie) {
	return getCSRFWithCookies(t, h, path, nil)
}

func getCSRFWithCookies(t *testing.T, h http.Handler, _ string, in []*http.Cookie) (string, []*http.Cookie) {
	t.Helper()
	return getAPICSRF(t, h, in)
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
