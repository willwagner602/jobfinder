package admin

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
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
	mux.HandleFunc("POST /admin/users/approve", s.requireAdmin(s.approveUser))
	mux.HandleFunc("POST /admin/users/suspend", s.requireAdmin(s.suspendUser))
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
	render(w, usersTemplate, map[string]any{"Users": users})
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

func (s *Server) tokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.Store.ListTokens(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	render(w, tokensTemplate, map[string]any{"Tokens": tokens})
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
	render(w, historyTemplate, map[string]any{"History": history, "Counts": counts})
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

const usersTemplate = `<!doctype html><title>Admin users</title><h1>Admin users</h1><nav><a href="/admin/tokens">Tokens</a> <a href="/admin/process-history">Process history</a></nav><table><tr><th>Email</th><th>Status</th><th>Confirmed</th><th>Actions</th></tr>{{range .Users}}<tr><td>{{.Email}}</td><td>{{.Status}}</td><td>{{.EmailConfirmedAt.String}}</td><td><form method="post" action="/admin/users/approve"><input type="hidden" name="user_id" value="{{.ID}}"><button>Approve</button></form><form method="post" action="/admin/users/suspend"><input type="hidden" name="user_id" value="{{.ID}}"><button>Suspend</button></form></td></tr>{{end}}</table>`

const tokensTemplate = `<!doctype html><title>Admin tokens</title><h1>Admin tokens</h1><nav><a href="/admin">Users</a> <a href="/admin/process-history">Process history</a></nav><form method="post" action="/admin/tokens/issue"><input name="subject" placeholder="subject"><input name="issued_to" placeholder="recipient email"><input name="ttl_hours" value="24"><label><input type="checkbox" name="send_email" value="true"> email via jobs@willwagner.me outbox</label><button>Issue token</button></form><table><tr><th>ID</th><th>Subject</th><th>Issued to</th><th>Expires</th><th>Revoked</th><th>Actions</th></tr>{{range .Tokens}}<tr><td>{{.ID}}</td><td>{{.Subject}}</td><td>{{.IssuedTo}}</td><td>{{.ExpiresAt}}</td><td>{{.RevokedAt.String}}</td><td><form method="post" action="/admin/tokens/revoke"><input type="hidden" name="token_id" value="{{.ID}}"><button>Revoke</button></form></td></tr>{{end}}</table>`

const historyTemplate = `<!doctype html><title>Process history</title><h1>Process history</h1><nav><a href="/admin">Users</a> <a href="/admin/tokens">Tokens</a></nav><h2>Status counts</h2><ul>{{range .Counts}}<li>{{.Status}}: {{.Count}}</li>{{end}}</ul><h2>Recent runs</h2><table><tr><th>Service</th><th>Status</th><th>Started</th><th>Finished</th><th>Items</th><th>Error details</th></tr>{{range .History}}<tr><td>{{.Service}}</td><td>{{.Status}}</td><td>{{.StartedAt}}</td><td>{{.FinishedAt.String}}</td><td>{{.ItemsProcessed}}</td><td><pre>{{.ErrorJSON.String}}</pre></td></tr>{{end}}</table>`
