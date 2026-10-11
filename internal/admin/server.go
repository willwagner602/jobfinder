package admin

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"jobs.willwagner.me/internal/schema"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type SMTPMailer struct {
	Addr     string
	Username string
	Password string
	From     string
}

func (m SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	if m.Addr == "" || m.From == "" {
		return fmt.Errorf("smtp address and from address are required")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	var auth smtp.Auth
	if m.Username != "" || m.Password != "" {
		host := m.Addr
		if i := strings.LastIndex(host, ":"); i > -1 {
			host = host[:i]
		}
		auth = smtp.PlainAuth("", m.Username, m.Password, host)
	}
	msg := []byte("From: " + m.From + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\n\r\n" + body + "\r\n")
	return smtp.SendMail(m.Addr, auth, m.From, []string{to}, msg)
}

type Server struct {
	Store      Store
	AdminToken string
	Mailer     Mailer
}

func NewServer(conn *sql.DB, adminToken string, mailer Mailer) *Server {
	return &Server{Store: Store{DB: conn}, AdminToken: adminToken, Mailer: mailer}
}

func NewSMTPMailerFromEnv() SMTPMailer {
	return SMTPMailer{
		Addr:     os.Getenv("JOBS_SMTP_ADDR"),
		Username: os.Getenv("JOBS_SMTP_USERNAME"),
		Password: os.Getenv("JOBS_SMTP_PASSWORD"),
		From:     envOrDefault("JOBS_SMTP_FROM", "jobs@willwagner.me"),
	}
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /admin", s.requireAdmin(s.users))
	mux.HandleFunc("GET /admin/users/{user_id}", s.requireAdmin(s.userDetail))
	mux.HandleFunc("GET /admin/users/approve", s.requireAdmin(methodNotAllowed))
	mux.HandleFunc("GET /admin/users/suspend", s.requireAdmin(methodNotAllowed))
	mux.HandleFunc("GET /admin/users/tombstone", s.requireAdmin(methodNotAllowed))
	mux.HandleFunc("GET /admin/users/untombstone", s.requireAdmin(methodNotAllowed))
	mux.HandleFunc("POST /admin/users/approve", s.requireAdmin(s.approveUser))
	mux.HandleFunc("POST /admin/users/suspend", s.requireAdmin(s.suspendUser))
	mux.HandleFunc("POST /admin/users/tombstone", s.requireAdmin(s.tombstoneUser))
	mux.HandleFunc("POST /admin/users/untombstone", s.requireAdmin(s.unTombstoneUser))
	mux.HandleFunc("POST /admin/signups", s.requireAdmin(s.setSignupDisabled))
	mux.HandleFunc("GET /admin/tokens", s.requireAdmin(s.tokens))
	mux.HandleFunc("POST /admin/tokens/issue", s.requireAdmin(s.issueToken))
	mux.HandleFunc("POST /admin/tokens/revoke", s.requireAdmin(s.revokeToken))
	mux.HandleFunc("GET /admin/process-history", s.requireAdmin(s.processHistory))
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" || got == r.Header.Get("Authorization") {
		return false
	}
	if s.AdminToken != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.AdminToken)) == 1 {
		return true
	}
	_, err := s.Store.ValidateToken(r.Context(), got)
	return err == nil
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderAdmin(w, usersTemplate, s.pageData(r.Context(), "users", "Admin users", map[string]any{"Users": users}))
}

func (s *Server) userDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "user_id must be a positive integer", http.StatusBadRequest)
		return
	}
	detail, err := s.loadUserDetail(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	renderAdmin(w, userDetailTemplate, s.pageData(r.Context(), "users", "User data", map[string]any{"Detail": detail}))
}

func (s *Server) approveUser(w http.ResponseWriter, r *http.Request) {
	s.changeUserStatus(w, r, true)
}

func (s *Server) suspendUser(w http.ResponseWriter, r *http.Request) {
	s.changeUserStatus(w, r, false)
}

func (s *Server) changeUserStatus(w http.ResponseWriter, r *http.Request, approve bool) {
	id, err := parseID(r, "user_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	adminEmail := r.FormValue("admin_email")
	if approve {
		err = s.Store.ApproveUser(r.Context(), id, adminEmail)
	} else {
		err = s.Store.SuspendUser(r.Context(), id, adminEmail)
	}
	if err != nil {
		status := http.StatusInternalServerError
		if err == ErrNotFound {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) tombstoneUser(w http.ResponseWriter, r *http.Request) {
	s.changeTombstoneStatus(w, r, true)
}

func (s *Server) unTombstoneUser(w http.ResponseWriter, r *http.Request) {
	s.changeTombstoneStatus(w, r, false)
}

func (s *Server) changeTombstoneStatus(w http.ResponseWriter, r *http.Request, tombstone bool) {
	id, err := parseID(r, "user_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	adminEmail := r.FormValue("admin_email")
	if tombstone {
		err = s.Store.TombstoneUser(r.Context(), id, adminEmail)
	} else {
		err = s.Store.UnTombstoneUser(r.Context(), id, adminEmail)
	}
	if err != nil {
		status := http.StatusInternalServerError
		if err == ErrNotFound {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) setSignupDisabled(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	want := r.FormValue("signups_disabled") == "true"
	if err := s.Store.SetSignupDisabled(r.Context(), want); err != nil {
		status := http.StatusInternalServerError
		if err == ErrNotFound {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	to := r.Header.Get("Referer")
	if to == "" {
		to = "/admin"
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (s *Server) tokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.Store.ListTokens(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderAdmin(w, tokensTemplate, s.pageData(r.Context(), "tokens", "Admin tokens", map[string]any{"Tokens": tokens}))
}

func (s *Server) issueToken(w http.ResponseWriter, r *http.Request) {
	ttlHours, err := strconv.Atoi(defaultString(r.FormValue("ttl_hours"), "24"))
	if err != nil || ttlHours <= 0 {
		http.Error(w, "ttl_hours must be a positive integer", http.StatusBadRequest)
		return
	}
	issued, err := s.Store.IssueToken(r.Context(), r.FormValue("subject"), r.FormValue("issued_to"), time.Duration(ttlHours)*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.Mailer != nil && r.FormValue("send_email") == "true" {
		body := fmt.Sprintf("Admin token for %s expires at %s UTC.\n\n%s\n", issued.Subject, issued.ExpiresAt.Format(time.RFC3339), issued.Token)
		if err := s.Mailer.Send(r.Context(), issued.IssuedTo, "jobs.willwagner.me admin token", body); err != nil {
			http.Error(w, "token issued but email send failed: "+err.Error(), http.StatusBadGateway)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(issued)
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "token_id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Store.RevokeToken(r.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if err == ErrNotFound {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	http.Redirect(w, r, "/admin/tokens", http.StatusSeeOther)
}

func (s *Server) processHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "100"))
	history, counts, err := s.Store.ProcessHistory(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderAdmin(w, historyTemplate, s.pageData(r.Context(), "process-history", "Process history", map[string]any{"History": history, "Counts": counts}))
}

func parseID(r *http.Request, name string) (int64, error) {
	if err := r.ParseForm(); err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(r.FormValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return id, nil
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func render(w http.ResponseWriter, tmpl string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.Must(template.New("page").Parse(tmpl)).Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type adminPage struct {
	Title           string
	Active          string
	SignupDisabled  bool
	SignupStatusErr string
	Data            map[string]any
}

func (s *Server) pageData(ctx context.Context, active, title string, data map[string]any) adminPage {
	disabled, err := s.Store.IsSignupDisabled(ctx)
	page := adminPage{Title: title, Active: active, SignupDisabled: disabled, Data: data}
	if err != nil {
		page.SignupStatusErr = err.Error()
	}
	return page
}

func renderAdmin(w http.ResponseWriter, content string, data adminPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.New("page").Funcs(template.FuncMap{
		"eq":           func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
		"statusAction": statusAction,
	}).Parse(adminLayout + content))
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func statusAction(status schema.UserStatus) string {
	if status == schema.UserStatusLive {
		return "Disable"
	}
	return "Enable"
}

type UserConfigDetail struct {
	UserID                int64
	ResumeText            sql.NullString
	ExperienceText        sql.NullString
	DesiredTitlesJSON     string
	SearchPreferencesJSON string
	BlockedCompaniesJSON  string
	MatcherName           sql.NullString
	CreatedAt             string
	UpdatedAt             string
}

type UserHistoryDetail struct {
	ID        int64
	UserID    sql.NullInt64
	EventType string
	InboundIP sql.NullString
	DataJSON  string
	CreatedAt string
}

type JobMatchDetail struct {
	ID                  int64
	UserID              int64
	JobListingID        int64
	JobListingVersion   int64
	Rating              sql.NullInt64
	Decision            string
	MatcherName         string
	MatcherMetadataJSON string
	JobMatchDataJSON    string
	CreatedAt           string
}

type UserDetail struct {
	User         User
	PasswordHash sql.NullString
	Config       *UserConfigDetail
	History      []UserHistoryDetail
	Matches      []JobMatchDetail
}

func (s *Server) loadUserDetail(ctx context.Context, id int64) (UserDetail, error) {
	var detail UserDetail
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT user_id, email, password_hash, status, email_confirmed_at, created_at, updated_at FROM users WHERE user_id = ?`, id).Scan(
		&detail.User.ID, &detail.User.Email, &detail.PasswordHash, &detail.User.Status, &detail.User.EmailConfirmedAt, &detail.User.CreatedAt, &detail.User.UpdatedAt,
	); err != nil {
		return UserDetail{}, err
	}
	var cfg UserConfigDetail
	err := s.Store.DB.QueryRowContext(ctx, `SELECT user_id, resume_text, experience_text, desired_titles_json, search_preferences_json, blocked_companies_json, matcher_name, created_at, updated_at FROM user_configs WHERE user_id = ?`, id).Scan(
		&cfg.UserID, &cfg.ResumeText, &cfg.ExperienceText, &cfg.DesiredTitlesJSON, &cfg.SearchPreferencesJSON, &cfg.BlockedCompaniesJSON, &cfg.MatcherName, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return UserDetail{}, err
	}
	if err == nil {
		detail.Config = &cfg
	}
	historyRows, err := s.Store.DB.QueryContext(ctx, `SELECT user_history_id, user_id, event_type, inbound_ip, data_json, created_at FROM user_history WHERE user_id = ? ORDER BY created_at DESC, user_history_id DESC`, id)
	if err != nil {
		return UserDetail{}, err
	}
	defer historyRows.Close()
	for historyRows.Next() {
		var row UserHistoryDetail
		if err := historyRows.Scan(&row.ID, &row.UserID, &row.EventType, &row.InboundIP, &row.DataJSON, &row.CreatedAt); err != nil {
			return UserDetail{}, err
		}
		detail.History = append(detail.History, row)
	}
	if err := historyRows.Err(); err != nil {
		return UserDetail{}, err
	}
	matchRows, err := s.Store.DB.QueryContext(ctx, `SELECT job_match_id, user_id, job_listing_id, job_listing_version, rating, decision, matcher_name, matcher_metadata_json, job_match_data_json, created_at FROM job_matches WHERE user_id = ? ORDER BY created_at DESC, job_match_id DESC LIMIT 25`, id)
	if err != nil {
		return UserDetail{}, err
	}
	defer matchRows.Close()
	for matchRows.Next() {
		var row JobMatchDetail
		if err := matchRows.Scan(&row.ID, &row.UserID, &row.JobListingID, &row.JobListingVersion, &row.Rating, &row.Decision, &row.MatcherName, &row.MatcherMetadataJSON, &row.JobMatchDataJSON, &row.CreatedAt); err != nil {
			return UserDetail{}, err
		}
		detail.Matches = append(detail.Matches, row)
	}
	return detail, matchRows.Err()
}

const adminLayout = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light; --bg:#f6f8fb; --panel:#ffffff; --panel-soft:#eef3f8; --text:#172033; --muted:#65748b; --line:#d9e2ec; --accent:#2754c5; --accent-soft:#e6edff; --danger:#b42318; --danger-soft:#fee4e2; --success:#147a45; --shadow:0 18px 40px rgba(22,34,51,.08); }
* { box-sizing:border-box; }
body { margin:0; min-height:100vh; font-family:Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:var(--bg); color:var(--text); }
body:has(#dark-mode-toggle:checked) { color-scheme:dark; --bg:#111827; --panel:#182235; --panel-soft:#233047; --text:#f8fafc; --muted:#b9c3d1; --line:#334155; --accent:#8fb4ff; --accent-soft:#1f355f; --danger:#ff9b93; --danger-soft:#4d2022; --success:#75d19b; --shadow:0 18px 40px rgba(0,0,0,.28); }
a { color:var(--accent); text-decoration:none; font-weight:650; }
a:hover { text-decoration:underline; }
.admin-shell { display:flex; min-height:100vh; }
.sidebar-toggle { position:absolute; opacity:0; pointer-events:none; }
.sidebar { width:280px; flex:0 0 280px; padding:1rem; background:var(--panel); border-right:1px solid var(--line); box-shadow:var(--shadow); transition:margin-left .18s ease; }
.sidebar h1 { margin:.25rem 0 1rem; font-size:1.1rem; letter-spacing:.02em; }
.sidebar-collapse { display:inline-flex; align-items:center; gap:.35rem; cursor:pointer; color:var(--muted); font-weight:700; margin-bottom:.75rem; }
.sidebar nav { display:grid; gap:.45rem; margin:1rem 0; }
.sidebar nav a { display:block; padding:.7rem .8rem; border-radius:14px; color:var(--text); }
.sidebar nav a.active, .sidebar nav a:hover { background:var(--accent-soft); color:var(--accent); text-decoration:none; }
.sidebar-card { margin-top:1rem; padding:.85rem; border:1px solid var(--line); border-radius:16px; background:var(--panel-soft); }
.sidebar-card p { margin:.3rem 0 .7rem; color:var(--muted); font-size:.9rem; }
.theme-toggle { display:flex; align-items:center; justify-content:space-between; gap:.75rem; font-weight:700; }
.sidebar-toggle:checked ~ .sidebar { margin-left:-218px; }
.sidebar-toggle:checked ~ .sidebar .sidebar-content { opacity:0; pointer-events:none; }
.content { flex:1; min-width:0; padding:2rem; }
.card { background:var(--panel); border:1px solid var(--line); border-radius:22px; box-shadow:var(--shadow); padding:1.25rem; margin-bottom:1.25rem; }
.card h2, .card h3 { margin-top:0; }
.page-header { display:flex; align-items:flex-start; justify-content:space-between; gap:1rem; margin-bottom:1rem; }
.kicker { color:var(--muted); margin:.15rem 0 0; }
.table-wrap { overflow:auto; border:1px solid var(--line); border-radius:18px; }
table { width:100%; border-collapse:collapse; background:var(--panel); }
th, td { text-align:left; padding:.75rem .85rem; border-bottom:1px solid var(--line); vertical-align:top; }
th { background:var(--panel-soft); color:var(--muted); font-size:.78rem; text-transform:uppercase; letter-spacing:.04em; }
tr:last-child td { border-bottom:0; }
.actions { display:flex; flex-wrap:wrap; gap:.45rem; }
.inline-form { display:inline; }
button, .button { border:0; border-radius:12px; background:var(--accent); color:white; padding:.55rem .8rem; font-weight:800; cursor:pointer; }
button.secondary { background:var(--panel-soft); color:var(--text); border:1px solid var(--line); }
button.danger { background:var(--danger); color:white; }
button:hover, .button:hover { filter:brightness(.96); text-decoration:none; }
input { border:1px solid var(--line); background:var(--panel); color:var(--text); border-radius:12px; padding:.55rem .65rem; }
.badge { display:inline-flex; align-items:center; border-radius:999px; padding:.2rem .55rem; font-size:.82rem; font-weight:800; background:var(--panel-soft); color:var(--muted); }
.badge.live { color:var(--success); }
.badge.tombstoned, .badge.suspended { color:var(--danger); background:var(--danger-soft); }
.warning { padding:.8rem; border-radius:16px; background:var(--danger-soft); color:var(--danger); }
.hint { color:var(--muted); font-size:.92rem; }
pre { white-space:pre-wrap; word-break:break-word; margin:0; }
.grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(230px,1fr)); gap:1rem; }
.kv { padding:.75rem; border:1px solid var(--line); border-radius:16px; background:var(--panel-soft); }
.kv strong { display:block; color:var(--muted); font-size:.78rem; text-transform:uppercase; letter-spacing:.04em; margin-bottom:.25rem; }
@media (max-width:760px) { .admin-shell { display:block; } .sidebar { width:auto; min-height:auto; border-right:0; border-bottom:1px solid var(--line); } .sidebar-toggle:checked ~ .sidebar { margin-left:0; } .sidebar-toggle:checked ~ .sidebar .sidebar-content { display:none; } .content { padding:1rem; } }
</style>
</head>
<body>
<div class="admin-shell">
<input class="sidebar-toggle" type="checkbox" id="sidebar-collapsed">
<aside class="sidebar">
<label class="sidebar-collapse" for="sidebar-collapsed">☰ Menu</label>
<div class="sidebar-content">
<h1>Admin console</h1>
<nav aria-label="Admin sections">
<a {{if eq .Active "users"}}class="active"{{end}} href="/admin">Users</a>
<a {{if eq .Active "tokens"}}class="active"{{end}} href="/admin/tokens">Tokens</a>
<a {{if eq .Active "process-history"}}class="active"{{end}} href="/admin/process-history">Process history</a>
</nav>
<div class="sidebar-card">
<label class="theme-toggle" for="dark-mode-toggle"><span>Dark mode</span><input id="dark-mode-toggle" type="checkbox"></label>
</div>
<div class="sidebar-card">
<strong>Signup access</strong>
{{if .SignupStatusErr}}<p>Unable to read signup status: {{.SignupStatusErr}}</p>{{else}}<p>New signups are currently <strong>{{if .SignupDisabled}}disabled{{else}}enabled{{end}}</strong>.</p>{{end}}
<form method="post" action="/admin/signups">
<input type="hidden" name="signups_disabled" value="{{if .SignupDisabled}}false{{else}}true{{end}}">
<button class="secondary">{{if .SignupDisabled}}Enable signups{{else}}Disable signups{{end}}</button>
</form>
</div>
</div>
</aside>
<main class="content">
{{template "content" .Data}}
</main>
</div>
</body>
</html>`

const usersTemplate = `{{define "content"}}
<section class="page-header">
<div><h1>Admin users</h1><p class="kicker">Approve, suspend, tombstone, and inspect user records.</p></div>
</section>
<section class="card">
<h2>User management</h2>
<p class="warning"><strong>Suspend</strong> removes login access but preserves the email claim, so the same email cannot sign up again. <strong>Tombstone</strong> is the delete action: it blocks login and removes the email from active signup uniqueness so the address can be used for a new account.</p>
<div class="table-wrap"><table><tr><th>Email</th><th>Status</th><th>Confirmed</th><th>Created</th><th>Updated</th><th>Actions</th></tr>{{range .Users}}<tr><td><a href="/admin/users/{{.ID}}">{{.Email}}</a></td><td><span class="badge {{.Status}}">{{.Status}}</span></td><td>{{.EmailConfirmedAt.String}}</td><td>{{.CreatedAt}}</td><td>{{.UpdatedAt}}</td><td><div class="actions">{{if eq .Status "tombstoned"}}<form class="inline-form" method="post" action="/admin/users/untombstone"><input type="hidden" name="user_id" value="{{.ID}}"><button class="secondary">Restore suspended</button></form>{{else}}<form class="inline-form" method="post" action="{{if eq .Status "live"}}/admin/users/suspend{{else}}/admin/users/approve{{end}}"><input type="hidden" name="user_id" value="{{.ID}}"><button>{{statusAction .Status}}</button></form><form class="inline-form" method="post" action="/admin/users/tombstone"><input type="hidden" name="user_id" value="{{.ID}}"><button class="danger">Tombstone/delete</button></form>{{end}}</div></td></tr>{{end}}</table></div>
</section>
{{end}}`

const userDetailTemplate = `{{define "content"}}
<section class="page-header"><div><h1>User data</h1><p class="kicker"><a href="/admin">← Back to users</a></p></div></section>
<section class="card"><h2>{{.Detail.User.Email}}</h2><div class="grid"><div class="kv"><strong>user_id</strong>{{.Detail.User.ID}}</div><div class="kv"><strong>email</strong>{{.Detail.User.Email}}</div><div class="kv"><strong>password_hash</strong><pre>{{.Detail.PasswordHash.String}}</pre></div><div class="kv"><strong>email_confirmed_at</strong>{{.Detail.User.EmailConfirmedAt.String}}</div><div class="kv"><strong>status</strong><span class="badge {{.Detail.User.Status}}">{{.Detail.User.Status}}</span></div><div class="kv"><strong>created_at</strong>{{.Detail.User.CreatedAt}}</div><div class="kv"><strong>updated_at</strong>{{.Detail.User.UpdatedAt}}</div></div></section>
<section class="card"><h2>user_configs</h2>{{with .Detail.Config}}<div class="grid"><div class="kv"><strong>user_id</strong>{{.UserID}}</div><div class="kv"><strong>resume_text</strong><pre>{{.ResumeText.String}}</pre></div><div class="kv"><strong>experience_text</strong><pre>{{.ExperienceText.String}}</pre></div><div class="kv"><strong>desired_titles_json</strong><pre>{{.DesiredTitlesJSON}}</pre></div><div class="kv"><strong>search_preferences_json</strong><pre>{{.SearchPreferencesJSON}}</pre></div><div class="kv"><strong>blocked_companies_json</strong><pre>{{.BlockedCompaniesJSON}}</pre></div><div class="kv"><strong>matcher_name</strong>{{.MatcherName.String}}</div><div class="kv"><strong>created_at</strong>{{.CreatedAt}}</div><div class="kv"><strong>updated_at</strong>{{.UpdatedAt}}</div></div>{{else}}<p class="hint">No user_configs row exists for this user.</p>{{end}}</section>
<section class="card"><h2>user_history</h2><div class="table-wrap"><table><tr><th>user_history_id</th><th>user_id</th><th>event_type</th><th>inbound_ip</th><th>data_json</th><th>created_at</th></tr>{{range .Detail.History}}<tr><td>{{.ID}}</td><td>{{.UserID.Int64}}</td><td>{{.EventType}}</td><td>{{.InboundIP.String}}</td><td><pre>{{.DataJSON}}</pre></td><td>{{.CreatedAt}}</td></tr>{{end}}</table></div></section>
<section class="card"><h2>Recent job_matches</h2><p class="hint">Showing the 25 most recent matches for this user.</p><div class="table-wrap"><table><tr><th>job_match_id</th><th>user_id</th><th>job_listing_id</th><th>job_listing_version</th><th>rating</th><th>decision</th><th>matcher_name</th><th>matcher_metadata_json</th><th>job_match_data_json</th><th>created_at</th></tr>{{range .Detail.Matches}}<tr><td>{{.ID}}</td><td>{{.UserID}}</td><td>{{.JobListingID}}</td><td>{{.JobListingVersion}}</td><td>{{.Rating.Int64}}</td><td>{{.Decision}}</td><td>{{.MatcherName}}</td><td><pre>{{.MatcherMetadataJSON}}</pre></td><td><pre>{{.JobMatchDataJSON}}</pre></td><td>{{.CreatedAt}}</td></tr>{{end}}</table></div></section>
{{end}}`

const tokensTemplate = `{{define "content"}}
<section class="page-header"><div><h1>Admin tokens</h1><p class="kicker">Issue and revoke bearer tokens for admin access.</p></div></section>
<section class="card"><h2>Issue token</h2><form class="actions" method="post" action="/admin/tokens/issue"><input name="subject" placeholder="subject"><input name="issued_to" placeholder="recipient email"><input name="ttl_hours" value="24"><label><input type="checkbox" name="send_email" value="true"> email via jobs@willwagner.me outbox</label><button>Issue token</button></form></section>
<section class="card"><h2>Existing tokens</h2><div class="table-wrap"><table><tr><th>ID</th><th>Subject</th><th>Issued to</th><th>Expires</th><th>Revoked</th><th>Actions</th></tr>{{range .Tokens}}<tr><td>{{.ID}}</td><td>{{.Subject}}</td><td>{{.IssuedTo}}</td><td>{{.ExpiresAt}}</td><td>{{.RevokedAt.String}}</td><td><form class="inline-form" method="post" action="/admin/tokens/revoke"><input type="hidden" name="token_id" value="{{.ID}}"><button class="danger">Revoke</button></form></td></tr>{{end}}</table></div></section>
{{end}}`

const historyTemplate = `{{define "content"}}
<section class="page-header"><div><h1>Process history</h1><p class="kicker">Recent background processing runs and result counts.</p></div></section>
<section class="card"><h2>Status counts</h2><ul>{{range .Counts}}<li>{{.Status}}: {{.Count}}</li>{{end}}</ul></section>
<section class="card"><h2>Recent runs</h2><div class="table-wrap"><table><tr><th>Service</th><th>Status</th><th>Started</th><th>Finished</th><th>Items</th><th>Error details</th></tr>{{range .History}}<tr><td>{{.Service}}</td><td>{{.Status}}</td><td>{{.StartedAt}}</td><td>{{.FinishedAt.String}}</td><td>{{.ItemsProcessed}}</td><td><pre>{{.ErrorJSON.String}}</pre></td></tr>{{end}}</table></div></section>
{{end}}`
