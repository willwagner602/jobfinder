package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/schema"
)

const (
	sessionCookieName = "jobs_session"
	csrfCookieName    = "jobs_csrf"
	codeTTL           = 15 * time.Minute
	sessionTTL        = 14 * 24 * time.Hour
	pbkdf2Iterations  = 210000
	pbkdf2KeyLen      = 32
)

// Notifier is the narrow boundary for delivering signup/login codes. Production
// deployments should provide an implementation backed by configured mail.
type Notifier interface {
	SendCode(ctx context.Context, email, code string) error
}

// MatchRunner is the narrow boundary for kicking off a user-specific match run.
type MatchRunner interface {
	RunPriorWeek(ctx context.Context, userID int64, start, end time.Time) error
}

type LogNotifier struct{}

func (LogNotifier) SendCode(context.Context, string, string) error { return nil }

type NoopMatchRunner struct{}

func (NoopMatchRunner) RunPriorWeek(context.Context, int64, time.Time, time.Time) error { return nil }

type Server struct {
	DB          *sql.DB
	Notifier    Notifier
	MatchRunner MatchRunner
	Templates   *template.Template
	Assets      http.FileSystem
	SessionKey  []byte
	Now         func() time.Time
}

type user struct {
	ID             int64
	Email          string
	PasswordHash   sql.NullString
	EmailConfirmed sql.NullString
	Status         string
}

type pageData struct {
	Title   string
	User    *user
	Error   string
	Notice  string
	CSRF    string
	Jobs    []jobDigest
	Job     *jobDigest
	Profile *profileData
}

type profileData struct {
	Resume              string `json:"resumeText"`
	Experience          string `json:"experienceText"`
	DesiredTitles       string `json:"desiredTitles"`
	SearchPreferences   string `json:"searchPreferences"`
	BlockedCompanies    string `json:"blockedCompanies"`
	MatcherName         string `json:"matcherName"`
	LastMatchRunMessage string `json:"lastMatchRunMessage,omitempty"`
}

type jobDigest struct {
	MatchID      int64         `json:"matchId"`
	ListingID    int64         `json:"listingId"`
	Version      int           `json:"version"`
	Title        string        `json:"title"`
	Company      string        `json:"company"`
	CanonicalURL string        `json:"canonicalUrl"`
	Rating       sql.NullInt64 `json:"rating"`
	Decision     string        `json:"decision"`
	CreatedAt    string        `json:"createdAt"`
	Summary      string        `json:"summary"`
}

func New(databasePath, templateDir string, sessionKey []byte, notifier Notifier, runner MatchRunner) (*Server, error) {
	return NewWithAssets(databasePath, templateDir, "frontend/dist", sessionKey, notifier, runner)
}

func NewWithAssets(databasePath, templateDir, assetDir string, sessionKey []byte, notifier Notifier, runner MatchRunner) (*Server, error) {
	conn, err := db.Open(databasePath)
	if err != nil {
		return nil, err
	}
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := ensureWebStorage(context.Background(), conn); err != nil {
		conn.Close()
		return nil, err
	}
	tpl, err := ParseTemplates(templateDir)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if len(sessionKey) < 32 {
		conn.Close()
		return nil, errors.New("session key must be at least 32 bytes")
	}
	return &Server{DB: conn, Notifier: notifierOrDefault(notifier), MatchRunner: runnerOrDefault(runner), Templates: tpl, Assets: http.Dir(assetDir), SessionKey: sessionKey, Now: time.Now}, nil
}

func ParseTemplates(dir string) (*template.Template, error) {
	pattern := filepath.Join(dir, "*.html")
	return template.ParseGlob(pattern)
}

func ParseTemplatesFS(fsys fs.FS) (*template.Template, error) {
	return template.ParseFS(fsys, "*.html")
}

func notifierOrDefault(n Notifier) Notifier {
	if n == nil {
		return LogNotifier{}
	}
	return n
}

func runnerOrDefault(r MatchRunner) MatchRunner {
	if r == nil {
		return NoopMatchRunner{}
	}
	return r
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("GET /assets/", s.assetHandler())
	mux.HandleFunc("GET /api/session", s.apiSession)
	mux.HandleFunc("POST /api/signup", s.apiSignup)
	mux.HandleFunc("POST /api/verify", s.apiVerify)
	mux.HandleFunc("POST /api/login", s.apiLogin)
	mux.HandleFunc("POST /api/logout", s.requireAPIAuth(s.apiLogout))
	mux.HandleFunc("GET /api/dashboard", s.requireAPIAuth(s.apiDashboard))
	mux.HandleFunc("GET /api/profile", s.requireAPIAuth(s.apiProfile))
	mux.HandleFunc("PUT /api/profile", s.requireAPIAuth(s.apiUpdateProfile))
	mux.HandleFunc("GET /api/jobs", s.requireAPIAuth(s.apiJobsList))
	mux.HandleFunc("GET /api/jobs/", s.requireAPIAuth(s.apiJobDetail))
	mux.HandleFunc("GET /signup", s.serveSPA)
	mux.HandleFunc("POST /signup", s.signupPost)
	mux.HandleFunc("GET /verify", s.serveSPA)
	mux.HandleFunc("POST /verify", s.verifyPost)
	mux.HandleFunc("GET /login", s.serveSPA)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("POST /logout", s.requireAuth(s.logoutPost))
	mux.HandleFunc("GET /dashboard", s.serveSPA)
	mux.HandleFunc("GET /", s.serveSPA)
	mux.HandleFunc("GET /onboarding", s.serveSPA)
	mux.HandleFunc("POST /onboarding", s.requireAuth(s.onboardingPost))
	mux.HandleFunc("GET /jobs", s.serveSPA)
	mux.HandleFunc("GET /jobs/", s.serveSPA)
	return securityHeaders(mux)
}

func (s *Server) assetHandler() http.Handler {
	return http.FileServer(s.assets())
}

func (s *Server) assets() http.FileSystem {
	if s.Assets != nil {
		return s.Assets
	}
	return http.Dir("frontend/dist")
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if !isSPARoute(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	f, err := s.assets().Open("index.html")
	if err != nil {
		s.serveLegacyUserRoute(w, r)
		return
	}
	defer f.Close()
	s.csrf(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.Copy(w, f); err != nil {
		http.Error(w, "failed to serve application", http.StatusInternalServerError)
	}
}

func (s *Server) serveLegacyUserRoute(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/signup":
		s.signupForm(w, r)
	case r.URL.Path == "/verify":
		s.verifyForm(w, r)
	case r.URL.Path == "/login":
		s.loginForm(w, r)
	case r.URL.Path == "/dashboard", r.URL.Path == "/":
		s.requireAuth(s.home)(w, r)
	case r.URL.Path == "/onboarding":
		s.requireAuth(s.onboardingForm)(w, r)
	case r.URL.Path == "/jobs":
		s.requireAuth(s.jobsList)(w, r)
	case strings.HasPrefix(r.URL.Path, "/jobs/"):
		s.requireAuth(s.jobDetail)(w, r)
	default:
		http.NotFound(w, r)
	}
}

func isSPARoute(path string) bool {
	if path == "/" || path == "/dashboard" || path == "/signup" || path == "/verify" || path == "/login" || path == "/onboarding" || path == "/jobs" {
		return true
	}
	if strings.HasPrefix(path, "/jobs/") {
		return strings.TrimPrefix(path, "/jobs/") != ""
	}
	return false
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type apiUser struct {
	Email string `json:"email"`
}

type apiSessionResponse struct {
	Authenticated bool     `json:"authenticated"`
	CSRFToken     string   `json:"csrfToken"`
	User          *apiUser `json:"user,omitempty"`
}

type apiSignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type apiVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type apiLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
	SendCode bool   `json:"sendCode"`
}

type apiProfileRequest struct {
	Resume            string `json:"resumeText"`
	Experience        string `json:"experienceText"`
	DesiredTitles     string `json:"desiredTitles"`
	SearchPreferences string `json:"searchPreferences"`
	BlockedCompanies  string `json:"blockedCompanies"`
	MatcherName       string `json:"matcherName"`
}

type apiJobDigest struct {
	MatchID      int64  `json:"matchId"`
	ListingID    int64  `json:"listingId"`
	Version      int    `json:"version"`
	Title        string `json:"title"`
	Company      string `json:"company"`
	CanonicalURL string `json:"canonicalUrl"`
	Rating       *int64 `json:"rating"`
	Decision     string `json:"decision"`
	CreatedAt    string `json:"createdAt"`
	Summary      string `json:"summary"`
}

func apiJob(j jobDigest) apiJobDigest {
	out := apiJobDigest{MatchID: j.MatchID, ListingID: j.ListingID, Version: j.Version, Title: j.Title, Company: j.Company, CanonicalURL: j.CanonicalURL, Decision: j.Decision, CreatedAt: j.CreatedAt, Summary: j.Summary}
	if j.Rating.Valid {
		out.Rating = &j.Rating.Int64
	}
	return out
}

func apiJobs(jobs []jobDigest) []apiJobDigest {
	out := make([]apiJobDigest, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, apiJob(job))
	}
	return out
}

func (s *Server) apiSession(w http.ResponseWriter, r *http.Request) {
	resp := apiSessionResponse{Authenticated: false, CSRFToken: s.csrf(w, r)}
	if u, ok := s.currentUser(r); ok && u.Status == string(schema.UserStatusLive) && u.EmailConfirmed.Valid {
		resp.Authenticated = true
		resp.User = &apiUser{Email: u.Email}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) apiSignup(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		s.writeAPIError(w, http.StatusForbidden, "bad_csrf", "Bad CSRF token")
		return
	}
	var req apiSignupRequest
	if !s.readJSON(w, r, &req) {
		return
	}
	email := normalizeEmail(req.Email)
	if email == "" {
		s.writeAPIError(w, http.StatusBadRequest, "email_required", "Email is required")
		return
	}
	var hash sql.NullString
	if req.Password != "" {
		encoded, err := hashPassword(req.Password)
		if err != nil {
			s.writeAPIError(w, http.StatusInternalServerError, "password_setup_failed", "Password setup failed")
			return
		}
		hash = sql.NullString{String: encoded, Valid: true}
	}
	err := s.createSignupAndIssueCode(r.Context(), email, hash, clientIP(r), nil)
	if errors.Is(err, errSignupsDisabled) {
		s.writeAPIError(w, http.StatusForbidden, "signups_disabled", "Signups are currently disabled")
		return
	}
	if errors.Is(err, errEmailNotAvailable) {
		s.writeAPIError(w, http.StatusForbidden, "email_not_available", "Email is not available for signup")
		return
	}
	if errors.Is(err, errCodeDelivery) {
		s.writeAPIError(w, http.StatusInternalServerError, "verification_code_failed", "Verification code failed")
		return
	}
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "signup_failed", "Signup failed")
		return
	}
	s.writeJSON(w, http.StatusAccepted, map[string]string{"next": "verify", "email": email})
}

func (s *Server) apiVerify(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		s.writeAPIError(w, http.StatusForbidden, "bad_csrf", "Bad CSRF token")
		return
	}
	var req apiVerifyRequest
	if !s.readJSON(w, r, &req) {
		return
	}
	email := normalizeEmail(req.Email)
	code := strings.TrimSpace(req.Code)
	u, err := s.findUserByEmail(r.Context(), email)
	if err != nil || !s.verifyCode(r.Context(), u.ID, code, "signup") {
		s.writeAPIError(w, http.StatusBadRequest, "invalid_code", "Invalid or expired code")
		return
	}
	_, err = s.DB.ExecContext(r.Context(), `UPDATE users SET email_confirmed_at = ?, status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE user_id = ?`, s.Now().UTC().Format(time.RFC3339Nano), schema.UserStatusLive, u.ID)
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "verification_failed", "Verification failed")
		return
	}
	s.recordHistory(r.Context(), u.ID, "email_confirmed", clientIP(r), map[string]any{"purpose": "signup"})
	s.setSession(w, u.ID)
	s.writeJSON(w, http.StatusOK, map[string]any{"next": "onboarding", "user": apiUser{Email: u.Email}})
}

func (s *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		s.writeAPIError(w, http.StatusForbidden, "bad_csrf", "Bad CSRF token")
		return
	}
	var req apiLoginRequest
	if !s.readJSON(w, r, &req) {
		return
	}
	email := normalizeEmail(req.Email)
	u, err := s.findUserByEmail(r.Context(), email)
	if err != nil || u.Status != string(schema.UserStatusLive) || !u.EmailConfirmed.Valid {
		s.writeAPIError(w, http.StatusForbidden, "account_not_ready", "Account is not ready for login")
		return
	}
	if req.SendCode {
		if err := s.issueCode(r.Context(), email, "login", clientIP(r)); err != nil {
			s.writeAPIError(w, http.StatusInternalServerError, "code_failed", "Code failed")
			return
		}
		s.writeJSON(w, http.StatusAccepted, map[string]string{"status": "code_sent"})
		return
	}
	if code := strings.TrimSpace(req.Code); code != "" {
		if !s.verifyCode(r.Context(), u.ID, code, "login") {
			s.writeAPIError(w, http.StatusBadRequest, "invalid_code", "Invalid or expired code")
			return
		}
	} else if !u.PasswordHash.Valid || !verifyPassword(req.Password, u.PasswordHash.String) {
		s.writeAPIError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}
	s.recordHistory(r.Context(), u.ID, "login", clientIP(r), map[string]any{"method": "web"})
	s.setSession(w, u.ID)
	s.writeJSON(w, http.StatusOK, map[string]any{"next": "dashboard", "user": apiUser{Email: u.Email}})
}

func (s *Server) apiLogout(w http.ResponseWriter, r *http.Request, _ *user) {
	if !s.validCSRF(r) {
		s.writeAPIError(w, http.StatusForbidden, "bad_csrf", "Bad CSRF token")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) apiDashboard(w http.ResponseWriter, r *http.Request, u *user) {
	profile := s.profile(r.Context(), u.ID)
	jobs, err := s.loadJobs(r.Context(), u.ID, 0)
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "jobs_failed", "Jobs failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"user": apiUser{Email: u.Email}, "profile": profile, "recentJobs": apiJobs(jobs)})
}

func (s *Server) apiProfile(w http.ResponseWriter, r *http.Request, u *user) {
	s.writeJSON(w, http.StatusOK, map[string]any{"user": apiUser{Email: u.Email}, "profile": s.profile(r.Context(), u.ID)})
}

func (s *Server) apiUpdateProfile(w http.ResponseWriter, r *http.Request, u *user) {
	if !s.validCSRF(r) {
		s.writeAPIError(w, http.StatusForbidden, "bad_csrf", "Bad CSRF token")
		return
	}
	var req apiProfileRequest
	if !s.readJSON(w, r, &req) {
		return
	}
	resume := strings.TrimSpace(req.Resume)
	experience := strings.TrimSpace(req.Experience)
	if resume == "" && experience == "" {
		s.writeAPIError(w, http.StatusBadRequest, "profile_text_required", "Add a resume or experience bullets")
		return
	}
	profile := &profileData{Resume: resume, Experience: experience, DesiredTitles: jsonList(req.DesiredTitles), SearchPreferences: jsonObjectFromText(req.SearchPreferences), BlockedCompanies: jsonList(req.BlockedCompanies), MatcherName: strings.TrimSpace(req.MatcherName)}
	_, err := s.DB.ExecContext(r.Context(), `INSERT INTO user_configs(user_id, resume_text, experience_text, desired_titles_json, search_preferences_json, blocked_companies_json, matcher_name)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET resume_text=excluded.resume_text, experience_text=excluded.experience_text, desired_titles_json=excluded.desired_titles_json, search_preferences_json=excluded.search_preferences_json, blocked_companies_json=excluded.blocked_companies_json, matcher_name=excluded.matcher_name, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, u.ID, resume, experience, profile.DesiredTitles, profile.SearchPreferences, profile.BlockedCompanies, nullString(profile.MatcherName))
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "profile_save_failed", "Profile save failed")
		return
	}
	end := s.Now().UTC()
	start := end.AddDate(0, 0, -7)
	if err := s.MatchRunner.RunPriorWeek(r.Context(), u.ID, start, end); err != nil {
		s.writeAPIError(w, http.StatusBadGateway, "match_kickoff_failed", "Match kickoff failed")
		return
	}
	s.recordHistory(r.Context(), u.ID, "profile_saved", clientIP(r), map[string]any{"match_window_start": start.Format(time.RFC3339Nano), "match_window_end": end.Format(time.RFC3339Nano)})
	profile.LastMatchRunMessage = "Saved; prior-week match run requested."
	s.writeJSON(w, http.StatusOK, map[string]any{"profile": profile, "message": profile.LastMatchRunMessage})
}

func (s *Server) apiJobsList(w http.ResponseWriter, r *http.Request, u *user) {
	jobs, err := s.loadJobs(r.Context(), u.ID, 0)
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "jobs_failed", "Jobs failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"jobs": apiJobs(jobs)})
}

func (s *Server) apiJobDetail(w http.ResponseWriter, r *http.Request, u *user) {
	idText := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		s.writeAPIError(w, http.StatusNotFound, "job_not_found", "Job not found")
		return
	}
	jobs, err := s.loadJobs(r.Context(), u.ID, id)
	if err != nil {
		s.writeAPIError(w, http.StatusInternalServerError, "job_failed", "Job failed")
		return
	}
	if len(jobs) == 0 {
		s.writeAPIError(w, http.StatusNotFound, "job_not_found", "Job not found")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"job": apiJob(jobs[0])})
}

func (s *Server) signupForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "signup.html", pageData{Title: "Sign up", CSRF: s.csrf(w, r)})
}

func (s *Server) signupPost(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	email := normalizeEmail(r.FormValue("email"))
	password := r.FormValue("password")
	if email == "" {
		s.render(w, r, "signup.html", pageData{Title: "Sign up", Error: "Email is required", CSRF: s.csrf(w, r)})
		return
	}
	var hash sql.NullString
	if password != "" {
		encoded, err := hashPassword(password)
		if err != nil {
			http.Error(w, "password setup failed", http.StatusInternalServerError)
			return
		}
		hash = sql.NullString{String: encoded, Valid: true}
	}
	err := s.createSignupAndIssueCode(r.Context(), email, hash, clientIP(r), nil)
	if errors.Is(err, errSignupsDisabled) {
		w.WriteHeader(http.StatusForbidden)
		s.render(w, r, "signup.html", pageData{Title: "Sign up", Error: "Signups are currently disabled", CSRF: s.csrf(w, r)})
		return
	}
	if errors.Is(err, errEmailNotAvailable) {
		w.WriteHeader(http.StatusForbidden)
		s.render(w, r, "signup.html", pageData{Title: "Sign up", Error: "Email is not available for signup", CSRF: s.csrf(w, r)})
		return
	}
	if errors.Is(err, errCodeDelivery) {
		http.Error(w, "verification code failed", http.StatusInternalServerError)
		return
	}
	if err != nil {
		http.Error(w, "signup failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/verify?email="+urlQuery(email), http.StatusSeeOther)
}

var (
	errSignupsDisabled   = errors.New("signups disabled")
	errEmailNotAvailable = errors.New("email not available for signup")
	errCodeDelivery      = errors.New("verification code delivery failed")
)

func (s *Server) createSignupAndIssueCode(ctx context.Context, email string, hash sql.NullString, ip string, afterBegin func(context.Context) error) error {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if afterBegin != nil {
		if err := afterBegin(ctx); err != nil {
			return err
		}
	}
	var signupsDisabled int
	if err := conn.QueryRowContext(ctx, `SELECT signups_disabled FROM site_config WHERE config_id = 1`).Scan(&signupsDisabled); err != nil {
		return err
	}
	if signupsDisabled != 0 {
		return errSignupsDisabled
	}
	var existingStatus string
	err = conn.QueryRowContext(ctx, `SELECT status FROM users WHERE email = ?`, email).Scan(&existingStatus)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && existingStatus == string(schema.UserStatusTombstoned) {
		return errEmailNotAvailable
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO users(email, password_hash, status) VALUES (?, ?, ?) ON CONFLICT(email) WHERE status <> 'tombstoned' DO UPDATE SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`, email, nullable(hash), schema.UserStatusPending); err != nil {
		return err
	}
	code, err := s.storeCodeOnConn(ctx, conn, email, "signup", ip)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	// The signup/code rows are now durable. If external delivery fails, surface
	// the error without attempting rollback; the committed code will expire by
	// the normal auth-code expiry path.
	if err := s.Notifier.SendCode(ctx, email, code); err != nil {
		return fmt.Errorf("%w: %v", errCodeDelivery, err)
	}
	return nil
}

func (s *Server) issueCodeOnConn(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, email, purpose, ip string) error {
	code, err := s.storeCodeOnConn(ctx, q, email, purpose, ip)
	if err != nil {
		return err
	}
	return s.Notifier.SendCode(ctx, email, code)
}

func (s *Server) storeCodeOnConn(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, email, purpose, ip string) (string, error) {
	var userID int64
	if err := q.QueryRowContext(ctx, `SELECT user_id FROM users WHERE email = ?`, email).Scan(&userID); err != nil {
		return "", err
	}
	code, err := numericCode()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(code))
	expires := s.Now().UTC().Add(codeTTL)
	res, err := q.ExecContext(ctx, `INSERT INTO web_auth_codes(user_id, purpose, code_hash, expires_at) VALUES (?, ?, ?, ?)`, userID, purpose, hex.EncodeToString(sum[:]), expires.Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	codeID, _ := res.LastInsertId()
	data := map[string]any{"purpose": purpose, "code_id": codeID, "expires_at": expires.Format(time.RFC3339Nano)}
	if _, err := q.ExecContext(ctx, `INSERT INTO user_history(user_id, event_type, inbound_ip, data_json) VALUES (?, ?, ?, ?)`, userID, "auth_code_issued", nullString(ip), mustJSON(data)); err != nil {
		return "", err
	}
	return code, nil
}

func (s *Server) verifyForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "verify.html", pageData{Title: "Verify email", Notice: r.URL.Query().Get("email"), CSRF: s.csrf(w, r)})
}

func (s *Server) verifyPost(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	email := normalizeEmail(r.FormValue("email"))
	code := strings.TrimSpace(r.FormValue("code"))
	u, err := s.findUserByEmail(r.Context(), email)
	if err != nil || !s.verifyCode(r.Context(), u.ID, code, "signup") {
		s.render(w, r, "verify.html", pageData{Title: "Verify email", Error: "Invalid or expired code", Notice: email, CSRF: s.csrf(w, r)})
		return
	}
	_, err = s.DB.ExecContext(r.Context(), `UPDATE users SET email_confirmed_at = ?, status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE user_id = ?`, s.Now().UTC().Format(time.RFC3339Nano), schema.UserStatusLive, u.ID)
	if err != nil {
		http.Error(w, "verification failed", http.StatusInternalServerError)
		return
	}
	s.recordHistory(r.Context(), u.ID, "email_confirmed", clientIP(r), map[string]any{"purpose": "signup"})
	s.setSession(w, u.ID)
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login.html", pageData{Title: "Log in", CSRF: s.csrf(w, r)})
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	if !s.validCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	email := normalizeEmail(r.FormValue("email"))
	u, err := s.findUserByEmail(r.Context(), email)
	if err != nil || u.Status != string(schema.UserStatusLive) || !u.EmailConfirmed.Valid {
		s.render(w, r, "login.html", pageData{Title: "Log in", Error: "Account is not ready for login", CSRF: s.csrf(w, r)})
		return
	}
	if r.FormValue("send_code") != "" {
		if err := s.issueCode(r.Context(), email, "login", clientIP(r)); err != nil {
			http.Error(w, "code failed", http.StatusInternalServerError)
			return
		}
		s.render(w, r, "login.html", pageData{Title: "Log in", Notice: "Code sent", CSRF: s.csrf(w, r)})
		return
	}
	if code := strings.TrimSpace(r.FormValue("code")); code != "" {
		if !s.verifyCode(r.Context(), u.ID, code, "login") {
			s.render(w, r, "login.html", pageData{Title: "Log in", Error: "Invalid or expired code", CSRF: s.csrf(w, r)})
			return
		}
	} else if !u.PasswordHash.Valid || !verifyPassword(r.FormValue("password"), u.PasswordHash.String) {
		s.render(w, r, "login.html", pageData{Title: "Log in", Error: "Invalid email or password", CSRF: s.csrf(w, r)})
		return
	}
	s.recordHistory(r.Context(), u.ID, "login", clientIP(r), map[string]any{"method": "web"})
	s.setSession(w, u.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logoutPost(w http.ResponseWriter, r *http.Request, u *user) {
	if !s.validCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request, u *user) {
	s.render(w, r, "home.html", pageData{Title: "Dashboard", User: u, CSRF: s.csrf(w, r)})
}

func (s *Server) onboardingForm(w http.ResponseWriter, r *http.Request, u *user) {
	profile := s.profile(r.Context(), u.ID)
	s.render(w, r, "onboarding.html", pageData{Title: "Onboarding", User: u, Profile: profile, CSRF: s.csrf(w, r)})
}

func (s *Server) onboardingPost(w http.ResponseWriter, r *http.Request, u *user) {
	if !s.validCSRF(r) {
		http.Error(w, "bad csrf token", http.StatusForbidden)
		return
	}
	resume := strings.TrimSpace(r.FormValue("resume_text"))
	experience := strings.TrimSpace(r.FormValue("experience_text"))
	if resume == "" && experience == "" {
		s.render(w, r, "onboarding.html", pageData{Title: "Onboarding", User: u, Error: "Add a resume or experience bullets", Profile: &profileData{}, CSRF: s.csrf(w, r)})
		return
	}
	profile := &profileData{Resume: resume, Experience: experience, DesiredTitles: jsonList(r.FormValue("desired_titles")), SearchPreferences: jsonObjectFromText(r.FormValue("search_preferences")), BlockedCompanies: jsonList(r.FormValue("blocked_companies")), MatcherName: strings.TrimSpace(r.FormValue("matcher_name"))}
	_, err := s.DB.ExecContext(r.Context(), `INSERT INTO user_configs(user_id, resume_text, experience_text, desired_titles_json, search_preferences_json, blocked_companies_json, matcher_name)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET resume_text=excluded.resume_text, experience_text=excluded.experience_text, desired_titles_json=excluded.desired_titles_json, search_preferences_json=excluded.search_preferences_json, blocked_companies_json=excluded.blocked_companies_json, matcher_name=excluded.matcher_name, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, u.ID, resume, experience, profile.DesiredTitles, profile.SearchPreferences, profile.BlockedCompanies, nullString(profile.MatcherName))
	if err != nil {
		http.Error(w, "profile save failed", http.StatusInternalServerError)
		return
	}
	end := s.Now().UTC()
	start := end.AddDate(0, 0, -7)
	if err := s.MatchRunner.RunPriorWeek(r.Context(), u.ID, start, end); err != nil {
		http.Error(w, "match kickoff failed", http.StatusBadGateway)
		return
	}
	s.recordHistory(r.Context(), u.ID, "profile_saved", clientIP(r), map[string]any{"match_window_start": start.Format(time.RFC3339Nano), "match_window_end": end.Format(time.RFC3339Nano)})
	profile.LastMatchRunMessage = "Saved; prior-week match run requested."
	s.render(w, r, "onboarding.html", pageData{Title: "Onboarding", User: u, Notice: profile.LastMatchRunMessage, Profile: profile, CSRF: s.csrf(w, r)})
}

func (s *Server) jobsList(w http.ResponseWriter, r *http.Request, u *user) {
	jobs, err := s.loadJobs(r.Context(), u.ID, 0)
	if err != nil {
		http.Error(w, "jobs failed", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "jobs.html", pageData{Title: "Job matches", User: u, Jobs: jobs, CSRF: s.csrf(w, r)})
}

func (s *Server) jobDetail(w http.ResponseWriter, r *http.Request, u *user) {
	idText := strings.TrimPrefix(r.URL.Path, "/jobs/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	jobs, err := s.loadJobs(r.Context(), u.ID, id)
	if err != nil {
		http.Error(w, "job failed", http.StatusInternalServerError)
		return
	}
	if len(jobs) == 0 {
		http.NotFound(w, r)
		return
	}
	s.render(w, r, "job.html", pageData{Title: jobs[0].Title, User: u, Job: &jobs[0], CSRF: s.csrf(w, r)})
}

func (s *Server) requireAuth(next func(http.ResponseWriter, *http.Request, *user)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUser(r)
		if !ok || u.Status != string(schema.UserStatusLive) || !u.EmailConfirmed.Valid {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r, u)
	}
}

func (s *Server) requireAPIAuth(next func(http.ResponseWriter, *http.Request, *user)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUser(r)
		if !ok || u.Status != string(schema.UserStatusLive) || !u.EmailConfirmed.Valid {
			s.writeAPIError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		next(w, r, u)
	}
}

func (s *Server) currentUser(r *http.Request) (*user, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, false
	}
	uid, ok := s.verifySession(c.Value)
	if !ok {
		return nil, false
	}
	u, err := s.findUserByID(r.Context(), uid)
	return u, err == nil
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if data.CSRF == "" {
		data.CSRF = s.csrf(w, r)
	}
	if err := s.Templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template failed", http.StatusInternalServerError)
	}
}

func (s *Server) readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		s.writeAPIError(w, http.StatusBadRequest, "bad_json", "Request body must be valid JSON")
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		s.writeAPIError(w, http.StatusBadRequest, "bad_json", "Request body must contain one JSON document")
		return false
	}
	return true
}

func (s *Server) writeAPIError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, map[string]apiError{"error": {Code: code, Message: message}})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) findUserByEmail(ctx context.Context, email string) (*user, error) {
	var u user
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, email, password_hash, email_confirmed_at, status FROM users WHERE email = ?`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.EmailConfirmed, &u.Status)
	return &u, err
}

func (s *Server) findUserByID(ctx context.Context, id int64) (*user, error) {
	var u user
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, email, password_hash, email_confirmed_at, status FROM users WHERE user_id = ?`, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.EmailConfirmed, &u.Status)
	return &u, err
}

func (s *Server) issueCode(ctx context.Context, email, purpose, ip string) error {
	return s.issueCodeOnConn(ctx, s.DB, email, purpose, ip)
}

func (s *Server) verifyCode(ctx context.Context, userID int64, code, purpose string) bool {
	sum := sha256.Sum256([]byte(code))
	want := hex.EncodeToString(sum[:])
	now := s.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE web_auth_codes
		SET consumed_at = ?
		WHERE auth_code_id = (
			SELECT auth_code_id FROM web_auth_codes
			WHERE user_id = ? AND purpose = ? AND code_hash = ? AND consumed_at IS NULL AND expires_at >= ?
			ORDER BY auth_code_id DESC
			LIMIT 1
		) AND consumed_at IS NULL`, now, userID, purpose, want, now)
	if err != nil {
		return false
	}
	changed, err := res.RowsAffected()
	if err != nil || changed != 1 {
		return false
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_history(user_id, event_type, data_json) VALUES (?, 'auth_code_used', ?)`, userID, mustJSON(map[string]any{"purpose": purpose})); err != nil {
		return false
	}
	return tx.Commit() == nil
}

func ensureWebStorage(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS web_auth_codes (
		auth_code_id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
		purpose TEXT NOT NULL,
		code_hash TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		consumed_at TEXT,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_web_auth_codes_lookup ON web_auth_codes(user_id, purpose, code_hash, consumed_at, expires_at)`)
	return err
}

func (s *Server) recordHistory(ctx context.Context, userID int64, eventType, ip string, data map[string]any) error {
	raw, _ := json.Marshal(data)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO user_history(user_id, event_type, inbound_ip, data_json) VALUES (?, ?, ?, ?)`, userID, eventType, nullString(ip), string(raw))
	return err
}

func mustJSON(data map[string]any) string {
	raw, _ := json.Marshal(data)
	return string(raw)
}

func (s *Server) profile(ctx context.Context, userID int64) *profileData {
	p := &profileData{}
	_ = s.DB.QueryRowContext(ctx, `SELECT coalesce(resume_text,''), coalesce(experience_text,''), desired_titles_json, search_preferences_json, blocked_companies_json, coalesce(matcher_name,'') FROM user_configs WHERE user_id = ?`, userID).Scan(&p.Resume, &p.Experience, &p.DesiredTitles, &p.SearchPreferences, &p.BlockedCompanies, &p.MatcherName)
	return p
}

func (s *Server) loadJobs(ctx context.Context, userID, matchID int64) ([]jobDigest, error) {
	query := `SELECT m.job_match_id, l.job_listing_id, v.version, v.title, b.company, l.canonical_url, m.rating, m.decision, m.created_at, substr(v.body_text, 1, 600)
		FROM job_matches m
		JOIN job_listings l ON l.job_listing_id = m.job_listing_id
		JOIN job_listing_versions v ON v.job_listing_id = m.job_listing_id AND v.version = m.job_listing_version
		JOIN job_boards b ON b.board_id = l.board_id
		WHERE m.user_id = ?`
	args := []any{userID}
	if matchID != 0 {
		query += ` AND m.job_match_id = ?`
		args = append(args, matchID)
	}
	query += ` ORDER BY m.created_at DESC LIMIT 100`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []jobDigest
	for rows.Next() {
		var j jobDigest
		if err := rows.Scan(&j.MatchID, &j.ListingID, &j.Version, &j.Title, &j.Company, &j.CanonicalURL, &j.Rating, &j.Decision, &j.CreatedAt, &j.Summary); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *Server) setSession(w http.ResponseWriter, userID int64) {
	exp := s.Now().UTC().Add(sessionTTL).Unix()
	payload := fmt.Sprintf("%d:%d", userID, exp)
	sig := sign(s.SessionKey, payload)
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: base64.RawURLEncoding.EncodeToString([]byte(payload + ":" + sig)), Path: "/", Expires: time.Unix(exp, 0), HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (s *Server) verifySession(value string) (int64, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, false
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 {
		return 0, false
	}
	payload := parts[0] + ":" + parts[1]
	if subtle.ConstantTimeCompare([]byte(sign(s.SessionKey, payload)), []byte(parts[2])) != 1 {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || s.Now().UTC().Unix() > exp {
		return 0, false
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	return uid, err == nil
}

func (s *Server) csrf(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	tok := randomToken(24)
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	return tok
}

func (s *Server) validCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		token = r.FormValue("csrf")
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) == 1
}

func sign(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func hashPassword(password string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(password), saltBytes, pbkdf2Iterations, pbkdf2KeyLen)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations, base64.RawURLEncoding.EncodeToString(saltBytes), base64.RawURLEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 100000 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	if len(want) != pbkdf2KeyLen {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := 32
	numBlocks := (keyLen + hLen - 1) / hLen
	var out []byte
	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func numericCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func normalizeEmail(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

func nullable(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}

func jsonList(text string) string {
	items := strings.Split(text, "\n")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := strings.TrimSpace(item); s != "" {
			out = append(out, s)
		}
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

func jsonObjectFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "{}"
	}
	var obj map[string]any
	if json.Unmarshal([]byte(text), &obj) == nil {
		return text
	}
	raw, _ := json.Marshal(map[string]string{"notes": text})
	return string(raw)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func urlQuery(v string) string { return strings.ReplaceAll(v, "@", "%40") }
