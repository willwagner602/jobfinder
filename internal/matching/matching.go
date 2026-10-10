package matching

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"jobs.willwagner.me/internal/schema"
)

const ServiceName = "job-match-search"

// Classifier evaluates one user against one listing version. Implementations can
// call any provider; the engine only depends on this provider-neutral contract.
type Classifier interface {
	Classify(context.Context, Request) (Result, error)
}

type Request struct {
	UserID         int64
	ResumeText     string
	ExperienceText string
	DesiredTitles  []string
	ListingID      int64
	Version        int
	Title          string
	Company        string
	BodyText       string
}

type Result struct {
	Decision schema.JobMatchDecision
	Rating   int
	Data     json.RawMessage
	Metadata json.RawMessage
}

type Options struct {
	DefaultMatcher string
	MaxAttempts    int
	InitialBackoff time.Duration
	Sleep          func(context.Context, time.Duration) error
	Now            func() time.Time
}

type Summary struct {
	Processed int
	Matched   int
	NoMatch   int
	Failures  int
}

type Engine struct {
	DB          *sql.DB
	Classifiers map[string]Classifier
	Options     Options
}

func (e Engine) Run(ctx context.Context) (Summary, error) {
	if e.DB == nil {
		return Summary{}, errors.New("matching database is required")
	}
	opts := e.options()
	if _, ok := e.Classifiers[opts.DefaultMatcher]; opts.DefaultMatcher == "" || !ok {
		return Summary{}, fmt.Errorf("default matcher %q is not registered", opts.DefaultMatcher)
	}
	started := opts.Now().UTC()
	summary := Summary{}
	runErr := e.run(ctx, opts, &summary)
	status := schema.ProcessStatusSuccess
	var errJSON any
	if runErr != nil {
		status = schema.ProcessStatusFailure
		errJSON = map[string]string{"error": runErr.Error()}
	} else if summary.Failures > 0 {
		status = schema.ProcessStatusPartial
		errJSON = map[string]int{"classifier_failures": summary.Failures}
	}
	if histErr := e.insertHistory(ctx, started, opts.Now().UTC(), status, summary, errJSON); histErr != nil {
		if runErr != nil {
			return summary, fmt.Errorf("%w; record history: %v", runErr, histErr)
		}
		return summary, histErr
	}
	return summary, runErr
}

func (e Engine) run(ctx context.Context, opts Options, summary *Summary) error {
	users, err := e.loadUsers(ctx)
	if err != nil {
		return err
	}
	listings, err := e.loadListings(ctx)
	if err != nil {
		return err
	}
	for _, user := range users {
		matcher := opts.DefaultMatcher
		if user.MatcherName != "" {
			matcher = user.MatcherName
		}
		classifier, ok := e.Classifiers[matcher]
		if !ok {
			return fmt.Errorf("user %d requested unregistered matcher %q", user.ID, matcher)
		}
		for _, listing := range listings {
			if alreadyMatched(ctx, e.DB, user.ID, listing.ID, listing.Version, matcher) {
				continue
			}
			req := Request{
				UserID: user.ID, ResumeText: user.ResumeText, ExperienceText: user.ExperienceText,
				DesiredTitles: user.DesiredTitles, ListingID: listing.ID, Version: listing.Version,
				Title: listing.Title, Company: listing.Company, BodyText: listing.BodyText,
			}
			result, err := preclassifiedNoMatch(user, listing)
			if err == nil && result.Decision == "" {
				result, err = e.classifyWithRetry(ctx, classifier, req, opts)
			}
			if err != nil {
				result = Result{Decision: schema.JobMatchDecisionFailure, Data: mustJSON(map[string]string{"error": err.Error()})}
			}
			if err := e.persist(ctx, user.ID, listing.ID, listing.Version, matcher, result); err != nil {
				return err
			}
			summary.Processed++
			switch result.Decision {
			case schema.JobMatchDecisionMatch:
				summary.Matched++
			case schema.JobMatchDecisionNoMatch:
				summary.NoMatch++
			case schema.JobMatchDecisionFailure:
				summary.Failures++
			}
		}
	}
	return nil
}

func (e Engine) classifyWithRetry(ctx context.Context, c Classifier, req Request, opts Options) (Result, error) {
	var last error
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		res, err := c.Classify(ctx, req)
		if err == nil {
			if err := validateResult(res); err != nil {
				return Result{}, err
			}
			return res, nil
		}
		last = err
		if attempt < opts.MaxAttempts {
			if err := opts.Sleep(ctx, opts.InitialBackoff*time.Duration(1<<(attempt-1))); err != nil {
				return Result{}, err
			}
		}
	}
	return Result{}, fmt.Errorf("classifier failed after %d attempts: %w", opts.MaxAttempts, last)
}

func validateResult(res Result) error {
	switch res.Decision {
	case schema.JobMatchDecisionMatch:
		if res.Rating < 1 || res.Rating > 5 {
			return fmt.Errorf("match rating %d outside 1-5", res.Rating)
		}
	case schema.JobMatchDecisionNoMatch:
		res.Rating = 0
	case schema.JobMatchDecisionFailure:
		return errors.New("classifier returned failure instead of error")
	default:
		return fmt.Errorf("invalid classifier decision %q", res.Decision)
	}
	return nil
}

type userConfig struct {
	ID               int64
	ResumeText       string
	ExperienceText   string
	DesiredTitles    []string
	BlockedCompanies []string
	MatcherName      string
}

type listingVersion struct {
	ID       int64
	Version  int
	Title    string
	Company  string
	BodyText string
}

func (e Engine) loadUsers(ctx context.Context) ([]userConfig, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT u.user_id, coalesce(c.resume_text,''), coalesce(c.experience_text,''), coalesce(c.desired_titles_json,'[]'), coalesce(c.blocked_companies_json,'[]'), coalesce(c.matcher_name,'')
		FROM users u JOIN user_configs c ON c.user_id = u.user_id WHERE u.status = ?`, schema.UserStatusLive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []userConfig
	for rows.Next() {
		var u userConfig
		var desired, blocked string
		if err := rows.Scan(&u.ID, &u.ResumeText, &u.ExperienceText, &desired, &blocked, &u.MatcherName); err != nil {
			return nil, err
		}
		u.DesiredTitles = decodeStringList(desired)
		u.BlockedCompanies = decodeStringList(blocked)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (e Engine) loadListings(ctx context.Context) ([]listingVersion, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT l.job_listing_id, l.current_version, v.title, b.company, v.body_text
		FROM job_listings l JOIN job_listing_versions v ON v.job_listing_id = l.job_listing_id AND v.version = l.current_version
		JOIN job_boards b ON b.board_id = l.board_id WHERE l.current_version > 0 ORDER BY l.job_listing_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []listingVersion
	for rows.Next() {
		var l listingVersion
		if err := rows.Scan(&l.ID, &l.Version, &l.Title, &l.Company, &l.BodyText); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func preclassifiedNoMatch(user userConfig, listing listingVersion) (Result, error) {
	for _, blocked := range user.BlockedCompanies {
		if strings.EqualFold(strings.TrimSpace(blocked), listing.Company) {
			return Result{Decision: schema.JobMatchDecisionNoMatch, Data: mustJSON(map[string]string{"filtered_by": "blocked_company"})}, nil
		}
	}
	if len(user.DesiredTitles) == 0 {
		return Result{}, nil
	}
	title := strings.ToLower(listing.Title)
	for _, desired := range user.DesiredTitles {
		if desired = strings.TrimSpace(strings.ToLower(desired)); desired != "" && strings.Contains(title, desired) {
			return Result{}, nil
		}
	}
	return Result{Decision: schema.JobMatchDecisionNoMatch, Data: mustJSON(map[string]string{"filtered_by": "desired_titles"})}, nil
}

func (e Engine) persist(ctx context.Context, userID, listingID int64, version int, matcher string, result Result) error {
	data := result.Data
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	metadata := result.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	var rating any
	if result.Decision == schema.JobMatchDecisionMatch {
		rating = result.Rating
	}
	_, err := e.DB.ExecContext(ctx, `INSERT INTO job_matches (user_id, job_listing_id, job_listing_version, rating, decision, matcher_name, matcher_metadata_json, job_match_data_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, job_listing_id, job_listing_version, matcher_name) DO UPDATE SET
			rating = excluded.rating, decision = excluded.decision, matcher_metadata_json = excluded.matcher_metadata_json, job_match_data_json = excluded.job_match_data_json`,
		userID, listingID, version, rating, result.Decision, matcher, string(metadata), string(data))
	return err
}

func (e Engine) insertHistory(ctx context.Context, started, finished time.Time, status schema.ProcessStatus, summary Summary, errData any) error {
	data := mustJSON(summary)
	var errJSON any
	if errData != nil {
		errJSON = string(mustJSON(errData))
	}
	_, err := e.DB.ExecContext(ctx, `INSERT INTO job_history (service, started_at, finished_at, status, items_processed, data_json, error_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ServiceName, started.Format(time.RFC3339Nano), finished.Format(time.RFC3339Nano), status, summary.Processed, string(data), errJSON)
	return err
}

func (e Engine) options() Options {
	o := e.Options
	if o.DefaultMatcher == "" {
		o.DefaultMatcher = "jev"
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 3
	}
	if o.InitialBackoff <= 0 {
		o.InitialBackoff = time.Second
	}
	if o.Sleep == nil {
		o.Sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	if o.Now == nil {
		o.Now = func() time.Time { return time.Now().UTC() }
	}
	return o
}

func alreadyMatched(ctx context.Context, conn *sql.DB, userID, listingID int64, version int, matcher string) bool {
	var n int
	_ = conn.QueryRowContext(ctx, `SELECT count(*) FROM job_matches WHERE user_id = ? AND job_listing_id = ? AND job_listing_version = ? AND matcher_name = ?`, userID, listingID, version, matcher).Scan(&n)
	return n > 0
}

func decodeStringList(s string) []string {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
