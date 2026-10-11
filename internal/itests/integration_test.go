package itests

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"jobs.willwagner.me/internal/admin"
	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/schema"
)

const adminToken = "itests-admin-token"

var built = struct {
	once  sync.Once
	web   string
	admin string
	err   error
}{}

func TestSignupVerifyLoginLogoutHappyPath(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer h.Close()

	client := newClient(t)
	email := "happy@example.com"
	password := "correct horse battery staple"
	getCSRF(t, client, h.webURL("/signup"))
	res := postForm(t, client, h.webURL("/signup"), url.Values{"email": {email}, "password": {password}, "csrf": {csrfFromJar(t, client, h.webBase)}})
	assertStatus(t, res, http.StatusSeeOther)
	assertLocation(t, res, "/verify?email=happy%40example.com")

	code := h.smtp.CodeFor(t, email)
	getCSRF(t, client, h.webURL("/verify?email=happy%40example.com"))
	res = postForm(t, client, h.webURL("/verify"), url.Values{"email": {email}, "code": {code}, "csrf": {csrfFromJar(t, client, h.webBase)}})
	assertStatus(t, res, http.StatusSeeOther)
	assertLocation(t, res, "/onboarding")

	res = get(t, client, h.webURL("/"))
	assertStatus(t, res, http.StatusOK)
	assertBodyContains(t, res, email)

	getCSRF(t, client, h.webURL("/"))
	res = postForm(t, client, h.webURL("/logout"), url.Values{"csrf": {csrfFromJar(t, client, h.webBase)}})
	assertStatus(t, res, http.StatusSeeOther)
	assertLocation(t, res, "/login")

	res = getNoRedirect(t, client, h.webURL("/"))
	assertStatus(t, res, http.StatusSeeOther)
	assertLocation(t, res, "/login")
}

func TestLoginJobsListAndDetailShowSeededListing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer h.Close()
	userID := signupVerifiedUser(t, h, "jobs@example.com", "correct horse battery staple")
	matchID := h.insertMatch(t, userID, "Principal Platform Engineer", "ExampleCo", "Build reliable systems")

	client := newClient(t)
	loginWithPassword(t, client, h, "jobs@example.com", "correct horse battery staple")
	res := get(t, client, h.webURL("/jobs"))
	assertStatus(t, res, http.StatusOK)
	assertBodyContains(t, res, "Principal Platform Engineer", "ExampleCo")

	res = get(t, client, h.webURL("/jobs/"+strconv.FormatInt(matchID, 10)))
	assertStatus(t, res, http.StatusOK)
	assertBodyContains(t, res, "Principal Platform Engineer", "Build reliable systems")
}

func TestAdminListUsersRequiresAuthAndBearerTokenListsUsers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer h.Close()
	signupVerifiedUser(t, h, "listed@example.com", "password123456")

	client := newClient(t)
	res := getNoRedirect(t, client, h.adminURL("/admin"))
	assertStatus(t, res, http.StatusUnauthorized)

	res = getAdmin(t, client, h.adminURL("/admin"))
	assertStatus(t, res, http.StatusOK)
	assertBodyContains(t, res, "listed@example.com", string(schema.UserStatusLive))
}

func TestSuspendRejectsLoginAndApproveRestoresLogin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer h.Close()
	userID := signupVerifiedUser(t, h, "disable@example.com", "correct horse battery staple")

	client := newClient(t)
	res := postAdminForm(t, client, h.adminURL("/admin/users/suspend"), url.Values{"user_id": {strconv.FormatInt(userID, 10)}, "admin_email": {"ops@example.com"}})
	assertStatus(t, res, http.StatusSeeOther)
	loginRejected(t, client, h, "disable@example.com", "correct horse battery staple", "Account is not ready for login")

	res = postAdminForm(t, client, h.adminURL("/admin/users/approve"), url.Values{"user_id": {strconv.FormatInt(userID, 10)}, "admin_email": {"ops@example.com"}})
	assertStatus(t, res, http.StatusSeeOther)
	loginWithPassword(t, newClient(t), h, "disable@example.com", "correct horse battery staple")
}

func TestTombstonedUserLoginRejectedAndSignupPolicyDocumented(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer h.Close()
	userID := signupVerifiedUser(t, h, "tombstone@example.com", "correct horse battery staple")
	h.setUserStatus(t, userID, schema.UserStatusTombstoned)

	loginRejected(t, newClient(t), h, "tombstone@example.com", "correct horse battery staple", "Account is not ready for login")

	client := newClient(t)
	getCSRF(t, client, h.webURL("/signup"))
	res := postForm(t, client, h.webURL("/signup"), url.Values{"email": {"tombstone@example.com"}, "password": {"new correct horse battery staple"}, "csrf": {csrfFromJar(t, client, h.webBase)}})
	assertStatus(t, res, http.StatusForbidden)
	assertBodyContains(t, res, "Email is not available for signup")
}

func TestAdminProcessHistoryPageRendersCounts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	defer h.Close()
	h.exec(t, `INSERT INTO job_history(service, started_at, finished_at, status, items_processed, error_json) VALUES ('job-match', '2025-01-01T00:00:00Z', '2025-01-01T00:01:00Z', 'failure', 3, '{"error":"boom"}')`)
	h.exec(t, `INSERT INTO job_history(service, started_at, finished_at, status, items_processed) VALUES ('board-search', '2025-01-02T00:00:00Z', '2025-01-02T00:01:00Z', 'success', 7)`)

	res := getAdmin(t, newClient(t), h.adminURL("/admin/process-history"))
	assertStatus(t, res, http.StatusOK)
	assertBodyContains(t, res, "failure: 1", "success: 1", "job-match", "boom", "7")
}

func TestSignupDisabledSiteConfigBlocksSignupWhenWebEnforcesIt(t *testing.T) {
	t.Parallel()
	h := newHarnessWithDBSetup(t, func(t *testing.T, conn *sql.DB) {
		_, err := conn.Exec(`UPDATE site_config SET signups_disabled = 1 WHERE config_id = 1`)
		if err != nil {
			t.Fatal(err)
		}
	})
	defer h.Close()

	client := newClient(t)
	getCSRF(t, client, h.webURL("/signup"))
	res := postForm(t, client, h.webURL("/signup"), url.Values{"email": {"closed@example.com"}, "password": {"correct horse battery staple"}, "csrf": {csrfFromJar(t, client, h.webBase)}})
	assertStatus(t, res, http.StatusForbidden)
	assertBodyContains(t, res, "Signups are currently disabled")
}

type harness struct {
	dir       string
	dbPath    string
	webBase   *url.URL
	adminBase *url.URL
	webCmd    *exec.Cmd
	adminCmd  *exec.Cmd
	smtp      *fakeSMTP
}

func newHarness(t *testing.T) *harness { return newHarnessWithDBSetup(t, nil) }

func newHarnessWithDBSetup(t *testing.T, setup func(*testing.T, *sql.DB)) *harness {
	t.Helper()
	webBin, adminBin := buildBinaries(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "jobs.sqlite3")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if err := admin.EnsureAdminTables(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	if setup != nil {
		setup(t, conn)
	}
	conn.Close()

	smtp := startFakeSMTP(t)
	webAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	h := &harness{dir: dir, dbPath: dbPath, webBase: mustURL("http://" + webAddr), adminBase: mustURL("http://" + adminAddr), smtp: smtp}
	h.webCmd = startProcess(t, webBin, []string{
		"JOBS_DB=" + dbPath,
		"WEB_ADDR=" + webAddr,
		"TEMPLATE_DIR=" + filepath.Join(repoRoot(t), "templates"),
		"SESSION_KEY=0123456789abcdef0123456789abcdef",
		"JOBS_SMTP_ADDR=" + smtp.addr,
		"JOBS_SMTP_USERNAME=user",
		"JOBS_SMTP_PASSWORD=pass",
		"JOBS_SMTP_FROM=jobs@willwagner.me",
	})
	h.adminCmd = startProcess(t, adminBin, []string{
		"JOBS_DB_PATH=" + dbPath,
		"JOBS_ADMIN_ADDR=" + adminAddr,
		"JOBS_ADMIN_TOKEN=" + adminToken,
	})
	waitHealth(t, h.webURL("/healthz"), http.StatusNoContent)
	waitHealth(t, h.adminURL("/healthz"), http.StatusOK)
	return h
}

func (h *harness) Close() {
	if h.webCmd != nil && h.webCmd.Process != nil {
		_ = h.webCmd.Process.Kill()
		_ = h.webCmd.Wait()
	}
	if h.adminCmd != nil && h.adminCmd.Process != nil {
		_ = h.adminCmd.Process.Kill()
		_ = h.adminCmd.Wait()
	}
	if h.smtp != nil {
		h.smtp.Close()
	}
}

func (h *harness) webURL(path string) string {
	return joinURL(h.webBase, path)
}
func (h *harness) adminURL(path string) string {
	return joinURL(h.adminBase, path)
}

func joinURL(base *url.URL, path string) string {
	rel, err := url.Parse(path)
	if err != nil {
		panic(err)
	}
	return base.ResolveReference(rel).String()
}

func (h *harness) openDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(h.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func (h *harness) exec(t *testing.T, query string, args ...any) sql.Result {
	t.Helper()
	conn := h.openDB(t)
	res, err := conn.Exec(query, args...)
	if err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
	return res
}

func (h *harness) setUserStatus(t *testing.T, userID int64, status schema.UserStatus) {
	t.Helper()
	h.exec(t, `UPDATE users SET status = ? WHERE user_id = ?`, status, userID)
}

func (h *harness) insertMatch(t *testing.T, userID int64, title, company, body string) int64 {
	t.Helper()
	conn := h.openDB(t)
	res, err := conn.Exec(`INSERT INTO job_boards(board_url, company, status) VALUES (?, ?, 'active')`, "https://example.com/jobs", company)
	if err != nil {
		t.Fatal(err)
	}
	boardID, _ := res.LastInsertId()
	listing, err := db.UpsertListingVersion(context.Background(), conn, db.ListingVersionInput{CanonicalURL: "https://example.com/jobs/1", BoardID: boardID, Title: title, Body: body, FoundAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	res, err = conn.Exec(`INSERT INTO job_matches(user_id, job_listing_id, job_listing_version, rating, decision, matcher_name) VALUES (?, ?, ?, 5, 'match', 'itests')`, userID, listing.ListingID, listing.Version)
	if err != nil {
		t.Fatal(err)
	}
	matchID, _ := res.LastInsertId()
	return matchID
}

func buildBinaries(t *testing.T) (string, string) {
	t.Helper()
	built.once.Do(func() {
		out := filepath.Join(os.TempDir(), "jobs-itests-bins")
		_ = os.MkdirAll(out, 0o755)
		built.web = filepath.Join(out, "web")
		built.admin = filepath.Join(out, "admin")
		root := repoRoot(t)
		for _, target := range []struct{ pkg, out string }{{"./cmd/web", built.web}, {"./cmd/admin", built.admin}} {
			cmd := exec.Command("go", "build", "-o", target.out, target.pkg)
			cmd.Dir = root
			if raw, err := cmd.CombinedOutput(); err != nil {
				built.err = fmt.Errorf("build %s: %w\n%s", target.pkg, err, raw)
				return
			}
		}
	})
	if built.err != nil {
		t.Fatal(built.err)
	}
	return built.web, built.admin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			t.Fatal("go.mod not found")
		}
	}
}

func startProcess(t *testing.T, bin string, env []string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = testLogWriter{t: t, prefix: filepath.Base(bin) + " stdout"}
	cmd.Stderr = testLogWriter{t: t, prefix: filepath.Base(bin) + " stderr"}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

type testLogWriter struct {
	t      *testing.T
	prefix string
}

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s: %s", w.prefix, strings.TrimSpace(string(p)))
	return len(p), nil
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func waitHealth(t *testing.T, target string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		res, err := http.Get(target)
		if err == nil {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode == want {
				return
			}
			last = fmt.Errorf("status %d", res.StatusCode)
		} else {
			last = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s not healthy: %v", target, last)
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
}

func get(t *testing.T, c *http.Client, target string) *http.Response {
	t.Helper()
	res, err := c.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
func getNoRedirect(t *testing.T, c *http.Client, target string) *http.Response {
	return get(t, c, target)
}
func getAdmin(t *testing.T, c *http.Client, target string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func postForm(t *testing.T, c *http.Client, target string, form url.Values) *http.Response {
	t.Helper()
	res, err := c.PostForm(target, form)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func postAdminForm(t *testing.T, c *http.Client, target string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func getCSRF(t *testing.T, c *http.Client, target string) {
	t.Helper()
	res := get(t, c, target)
	assertStatus(t, res, http.StatusOK)
	res.Body.Close()
}

func csrfFromJar(t *testing.T, c *http.Client, base *url.URL) string {
	t.Helper()
	for _, cookie := range c.Jar.Cookies(base) {
		if cookie.Name == "jobs_csrf" {
			return cookie.Value
		}
	}
	t.Fatal("jobs_csrf cookie not found")
	return ""
}

func signupVerifiedUser(t *testing.T, h *harness, email, password string) int64 {
	t.Helper()
	client := newClient(t)
	getCSRF(t, client, h.webURL("/signup"))
	res := postForm(t, client, h.webURL("/signup"), url.Values{"email": {email}, "password": {password}, "csrf": {csrfFromJar(t, client, h.webBase)}})
	assertStatus(t, res, http.StatusSeeOther)
	code := h.smtp.CodeFor(t, email)
	getCSRF(t, client, h.webURL("/verify?email="+url.QueryEscape(email)))
	res = postForm(t, client, h.webURL("/verify"), url.Values{"email": {email}, "code": {code}, "csrf": {csrfFromJar(t, client, h.webBase)}})
	assertStatus(t, res, http.StatusSeeOther)
	conn := h.openDB(t)
	var userID int64
	if err := conn.QueryRow(`SELECT user_id FROM users WHERE email = ? AND status = ?`, email, schema.UserStatusLive).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	return userID
}

func loginWithPassword(t *testing.T, c *http.Client, h *harness, email, password string) {
	t.Helper()
	getCSRF(t, c, h.webURL("/login"))
	res := postForm(t, c, h.webURL("/login"), url.Values{"email": {email}, "password": {password}, "csrf": {csrfFromJar(t, c, h.webBase)}})
	assertStatus(t, res, http.StatusSeeOther)
	assertLocation(t, res, "/")
}

func loginRejected(t *testing.T, c *http.Client, h *harness, email, password, want string) {
	t.Helper()
	getCSRF(t, c, h.webURL("/login"))
	res := postForm(t, c, h.webURL("/login"), url.Values{"email": {email}, "password": {password}, "csrf": {csrfFromJar(t, c, h.webBase)}})
	assertStatus(t, res, http.StatusOK)
	assertBodyContains(t, res, want)
}

func assertStatus(t *testing.T, res *http.Response, want int) {
	t.Helper()
	if res.StatusCode != want {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("status = %d, want %d body=%s", res.StatusCode, want, body)
	}
}

func assertLocation(t *testing.T, res *http.Response, want string) {
	t.Helper()
	if got := res.Header.Get("Location"); got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
}

func assertBodyContains(t *testing.T, res *http.Response, wants ...string) {
	t.Helper()
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("body missing %q in %s", want, text)
		}
	}
}

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

// fakeSMTP is a minimal AUTH-capable SMTP server for net/smtp.SendMail.
type fakeSMTP struct {
	addr     string
	listener net.Listener
	messages chan smtpMessage
	done     chan struct{}
}

type smtpMessage struct{ to, body string }

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{addr: ln.Addr().String(), listener: ln, messages: make(chan smtpMessage, 20), done: make(chan struct{})}
	go s.serve()
	return s
}

func (s *fakeSMTP) serve() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSMTP) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 fake ESMTP")
	var to string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "EHLO") || strings.HasPrefix(line, "HELO"):
			write("250-fake greets you")
			write("250 AUTH PLAIN")
		case strings.HasPrefix(line, "AUTH"):
			write("235 authenticated")
		case strings.HasPrefix(line, "MAIL FROM"):
			write("250 ok")
		case strings.HasPrefix(line, "RCPT TO"):
			to = line
			write("250 ok")
		case line == "DATA":
			write("354 go ahead")
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				trimmed := strings.TrimRight(dataLine, "\r\n")
				if trimmed == "." {
					break
				}
				body.WriteString(dataLine)
			}
			s.messages <- smtpMessage{to: to, body: body.String()}
			write("250 accepted")
		case strings.HasPrefix(line, "QUIT"):
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}

func (s *fakeSMTP) CodeFor(t *testing.T, email string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	re := regexp.MustCompile(`one-time code is: ([0-9]{6})`)
	for {
		select {
		case msg := <-s.messages:
			if !strings.Contains(msg.to, email) {
				continue
			}
			m := re.FindStringSubmatch(msg.body)
			if len(m) == 2 {
				return m[1]
			}
		case <-deadline:
			t.Fatal("timed out waiting for SMTP code for " + email)
		}
	}
}

func (s *fakeSMTP) Close() {
	_ = s.listener.Close()
	select {
	case <-s.done:
	case <-time.After(time.Second):
	}
}
