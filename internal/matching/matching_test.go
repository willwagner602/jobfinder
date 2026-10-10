package matching

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"jobs.willwagner.me/internal/db"
	"jobs.willwagner.me/internal/schema"
)

type scriptedClassifier struct {
	results []Result
	errs    []error
	calls   []Request
}

func (s *scriptedClassifier) Classify(_ context.Context, req Request) (Result, error) {
	s.calls = append(s.calls, req)
	i := len(s.calls) - 1
	if i < len(s.errs) && s.errs[i] != nil {
		return Result{}, s.errs[i]
	}
	if i < len(s.results) {
		return s.results[i], nil
	}
	return Result{Decision: schema.JobMatchDecisionNoMatch, Data: json.RawMessage(`{"default":true}`)}, nil
}

func TestMatcherSelectionUsesUserOverrideAndGlobalDefault(t *testing.T) {
	conn := testDB(t)
	listingID := seedListing(t, conn, "Backend Engineer", "Example Co", "Go services")
	seedUser(t, conn, "default@example.com", "", `[]`, `[]`)
	seedUser(t, conn, "override@example.com", "custom", `[]`, `[]`)
	jev := &scriptedClassifier{results: []Result{{Decision: schema.JobMatchDecisionNoMatch, Data: json.RawMessage(`{"matcher":"jev"}`)}}}
	custom := &scriptedClassifier{results: []Result{{Decision: schema.JobMatchDecisionMatch, Rating: 5, Data: json.RawMessage(`{"matcher":"custom"}`)}}}

	summary, err := Engine{DB: conn, Classifiers: map[string]Classifier{"jev": jev, "custom": custom}, Options: testOptions()}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Processed != 2 || summary.Matched != 1 || summary.NoMatch != 1 || summary.Failures != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(jev.calls) != 1 || len(custom.calls) != 1 {
		t.Fatalf("calls: jev=%d custom=%d", len(jev.calls), len(custom.calls))
	}
	assertMatch(t, conn, 1, listingID, "jev", schema.JobMatchDecisionNoMatch, 0)
	assertMatch(t, conn, 2, listingID, "custom", schema.JobMatchDecisionMatch, 5)
}

func TestHighLevelFiltersPersistNoMatchWithoutCallingClassifier(t *testing.T) {
	conn := testDB(t)
	listingID := seedListing(t, conn, "Sales Manager", "Blocked Co", "Sales role")
	seedUser(t, conn, "person@example.com", "", `["engineer"]`, `["Blocked Co"]`)
	classifier := &scriptedClassifier{}

	summary, err := Engine{DB: conn, Classifiers: map[string]Classifier{"jev": classifier}, Options: testOptions()}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Processed != 1 || summary.NoMatch != 1 || len(classifier.calls) != 0 {
		t.Fatalf("summary=%+v calls=%d", summary, len(classifier.calls))
	}
	assertMatch(t, conn, 1, listingID, "jev", schema.JobMatchDecisionNoMatch, 0)
}

func TestClassifierRetryBackoffThenPersistsMatch(t *testing.T) {
	conn := testDB(t)
	seedListing(t, conn, "Backend Engineer", "Example Co", "Go services")
	seedUser(t, conn, "person@example.com", "", `[]`, `[]`)
	classifier := &scriptedClassifier{
		errs:    []error{errors.New("temporary"), nil},
		results: []Result{{}, {Decision: schema.JobMatchDecisionMatch, Rating: 4, Data: json.RawMessage(`{"ok":true}`)}},
	}
	var sleeps []time.Duration
	opts := testOptions()
	opts.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }

	summary, err := Engine{DB: conn, Classifiers: map[string]Classifier{"jev": classifier}, Options: opts}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Matched != 1 || summary.Failures != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(sleeps) != 1 || sleeps[0] != 10*time.Millisecond {
		t.Fatalf("sleeps = %v", sleeps)
	}
	assertHistory(t, conn, schema.ProcessStatusSuccess, 1)
}

func TestExhaustedClassifierFailurePersistsFailureAndPartialHistory(t *testing.T) {
	conn := testDB(t)
	listingID := seedListing(t, conn, "Backend Engineer", "Example Co", "Go services")
	seedUser(t, conn, "person@example.com", "", `[]`, `[]`)
	classifier := &scriptedClassifier{errs: []error{errors.New("one"), errors.New("two"), errors.New("three")}}
	var sleeps []time.Duration
	opts := testOptions()
	opts.Sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }

	summary, err := Engine{DB: conn, Classifiers: map[string]Classifier{"jev": classifier}, Options: opts}.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Processed != 1 || summary.Failures != 1 || summary.Matched != 0 || summary.NoMatch != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	if len(sleeps) != 2 || sleeps[0] != 10*time.Millisecond || sleeps[1] != 20*time.Millisecond {
		t.Fatalf("sleeps = %v", sleeps)
	}
	assertMatch(t, conn, 1, listingID, "jev", schema.JobMatchDecisionFailure, 0)
	assertHistory(t, conn, schema.ProcessStatusPartial, 1)
}

func TestEngineValidationAndRetryAbortBranches(t *testing.T) {
	if _, err := (Engine{}).Run(context.Background()); err == nil {
		t.Fatal("nil DB accepted")
	}
	conn := testDB(t)
	if _, err := (Engine{DB: conn}).Run(context.Background()); err == nil {
		t.Fatal("missing default classifier accepted")
	}
	seedListing(t, conn, "Engineer", "Example", "body")
	seedUser(t, conn, "bad@example.com", "missing", `[]`, `[]`)
	if _, err := (Engine{DB: conn, Classifiers: map[string]Classifier{"jev": &scriptedClassifier{}}}).Run(context.Background()); err == nil {
		t.Fatal("unregistered user matcher accepted")
	}

	conn = testDB(t)
	seedListing(t, conn, "Engineer", "Example", "body")
	seedUser(t, conn, "retry@example.com", "", `[]`, `[]`)
	classifier := &scriptedClassifier{results: []Result{{Decision: schema.JobMatchDecisionMatch, Rating: 0}}}
	summary, err := (Engine{DB: conn, Classifiers: map[string]Classifier{"jev": classifier}, Options: testOptions()}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run invalid classifier result: %v", err)
	}
	if summary.Failures != 1 {
		t.Fatalf("summary=%+v want one persisted failure", summary)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := testOptions()
	opts.Sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	_, err = (Engine{DB: testDB(t), Classifiers: map[string]Classifier{"jev": &scriptedClassifier{errs: []error{errors.New("temporary")}}}, Options: opts}).Run(ctx)
	if err == nil {
		t.Fatal("cancelled retry accepted")
	}
}

func testOptions() Options {
	fixed := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	return Options{DefaultMatcher: "jev", MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, Sleep: func(context.Context, time.Duration) error { return nil }, Now: func() time.Time { return fixed }}
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.EnsureCurrent(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

func seedUser(t *testing.T, conn *sql.DB, email, matcher, desired, blocked string) int64 {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO users(email, status, email_confirmed_at) VALUES (?, ?, ?)`, email, schema.UserStatusLive, "2025-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := conn.Exec(`INSERT INTO user_configs(user_id, resume_text, experience_text, desired_titles_json, blocked_companies_json, matcher_name) VALUES (?, ?, ?, ?, ?, ?)`, id, "resume", "experience", desired, blocked, nullIfEmpty(matcher)); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedListing(t *testing.T, conn *sql.DB, title, company, body string) int64 {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO job_boards(board_url, company, ats_type, status) VALUES (?, ?, ?, ?)`, "https://"+company+"/jobs", company, "lever", schema.BoardStatusActive)
	if err != nil {
		t.Fatal(err)
	}
	boardID, _ := res.LastInsertId()
	version, err := db.UpsertListingVersion(context.Background(), conn, db.ListingVersionInput{CanonicalURL: "https://" + company + "/jobs/1", BoardID: boardID, Title: title, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	return version.ListingID
}

func assertMatch(t *testing.T, conn *sql.DB, userID, listingID int64, matcher string, decision schema.JobMatchDecision, rating int) {
	t.Helper()
	var gotDecision string
	var gotRating sql.NullInt64
	if err := conn.QueryRow(`SELECT decision, rating FROM job_matches WHERE user_id = ? AND job_listing_id = ? AND matcher_name = ?`, userID, listingID, matcher).Scan(&gotDecision, &gotRating); err != nil {
		t.Fatal(err)
	}
	if gotDecision != string(decision) {
		t.Fatalf("decision = %q, want %q", gotDecision, decision)
	}
	if rating == 0 {
		if gotRating.Valid {
			t.Fatalf("rating = %d, want null", gotRating.Int64)
		}
	} else if !gotRating.Valid || int(gotRating.Int64) != rating {
		t.Fatalf("rating = %v, want %d", gotRating, rating)
	}
}

func assertHistory(t *testing.T, conn *sql.DB, status schema.ProcessStatus, items int) {
	t.Helper()
	var gotStatus string
	var gotItems int
	if err := conn.QueryRow(`SELECT status, items_processed FROM job_history WHERE service = ? ORDER BY job_history_id DESC LIMIT 1`, ServiceName).Scan(&gotStatus, &gotItems); err != nil {
		t.Fatal(err)
	}
	if gotStatus != string(status) || gotItems != items {
		t.Fatalf("history status=%q items=%d", gotStatus, gotItems)
	}
}

func TestOptionsDefaults(t *testing.T) {
	o := (Engine{}).options()
	if o.DefaultMatcher != "jev" || o.MaxAttempts != 3 || o.InitialBackoff != time.Second || o.Sleep == nil || o.Now == nil {
		t.Fatalf("options defaults = %+v", o)
	}
	if err := o.Sleep(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := o.Sleep(ctx, time.Hour); err == nil {
		t.Fatal("cancelled sleep succeeded")
	}
}

func TestValidateResultBranches(t *testing.T) {
	for _, result := range []Result{{Decision: schema.JobMatchDecisionFailure}, {Decision: "bad"}, {Decision: schema.JobMatchDecisionMatch, Rating: 6}} {
		if err := validateResult(result); err == nil {
			t.Errorf("validateResult(%+v) succeeded", result)
		}
	}
	if err := validateResult(Result{Decision: schema.JobMatchDecisionNoMatch, Rating: 5}); err != nil {
		t.Fatal(err)
	}
	if err := validateResult(Result{Decision: schema.JobMatchDecisionMatch, Rating: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageHelpersAndFilters(t *testing.T) {
	if got := decodeStringList(`not json`); got != nil {
		t.Fatalf("invalid list = %v", got)
	}
	if got := decodeStringList(`["x"]`); len(got) != 1 || got[0] != "x" {
		t.Fatalf("decoded list = %v", got)
	}
	if string(mustJSON(make(chan int))) != `{}` {
		t.Fatal("unmarshalable value should produce empty object")
	}
	if string(mustJSON(map[string]int{"x": 1})) != `{"x":1}` {
		t.Fatal("marshalable value lost")
	}
	if _, err := (llmDecision{Decision: "match", Rating: 0}).result("m"); err == nil {
		t.Fatal("invalid match rating accepted")
	}
	if _, err := (llmDecision{Decision: "other"}).result("m"); err == nil {
		t.Fatal("invalid decision accepted")
	}
	if _, err := (llmDecision{Decision: "no_match", Rating: 4}).result("m"); err != nil {
		t.Fatal(err)
	}
	if got := truncateForError(" \nshort \t"); got != "short" {
		t.Fatalf("trimmed error = %q", got)
	}
	if got := truncateForError(strings.Repeat("x", 201)); len(got) != 203 || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncation = %q", got)
	}
	if got, _ := preclassifiedNoMatch(userConfig{BlockedCompanies: []string{" blocked co "}}, listingVersion{Company: "Blocked Co"}); got.Decision != schema.JobMatchDecisionNoMatch {
		t.Fatal("blocked company not filtered")
	}
	if got, _ := preclassifiedNoMatch(userConfig{DesiredTitles: []string{" Engineer "}}, listingVersion{Title: "Senior Engineer"}); got.Decision != "" {
		t.Fatal("matching desired title filtered")
	}
	if got, _ := preclassifiedNoMatch(userConfig{DesiredTitles: []string{"Engineer"}}, listingVersion{Title: "Designer"}); got.Decision != schema.JobMatchDecisionNoMatch {
		t.Fatal("nonmatching title accepted")
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
