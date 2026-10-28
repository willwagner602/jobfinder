package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"jobfinder/internal/db"
)

// Listing is a provider-neutral job description ready for database ingestion.
type Listing struct {
	CanonicalURL string
	Title        string
	Salary       string
	BodyHTML     string
	PostedAt     sql.NullTime
	Metadata     map[string]string
}

// Board identifies one configured job board row to scrape.
type Board struct {
	ID       int64
	URL      string
	Company  string
	ATSType  string
	Metadata string
}

type Provider interface {
	Fetch(context.Context, *http.Client, Board, time.Time) ([]Listing, error)
}

type Result struct {
	Seen     int
	Inserted int
	Updated  int
	Errors   []string
}

func ProviderFor(ats string) (Provider, bool) {
	switch normalizedATS(ats) {
	case "greenhouse":
		return GreenhouseProvider{}, true
	case "lever":
		return LeverProvider{}, true
	case "ashby":
		return AshbyProvider{}, true
	case "workday":
		return WorkdayProvider{}, true
	case "jobvite":
		return JobviteProvider{}, true
	case "climatebase":
		return ClimatebaseProvider{}, true
	case "hackernews", "hn":
		return HackerNewsProvider{}, true
	default:
		return nil, false
	}
}

func normalizedATS(ats string) string {
	return strings.ToLower(strings.TrimSpace(ats))
}

func isHackerNews(ats string) bool {
	switch normalizedATS(ats) {
	case "hackernews", "hn":
		return true
	default:
		return false
	}
}

func IngestBoard(ctx context.Context, conn *sql.DB, client *http.Client, board Board, since time.Time, provider Provider) (Result, error) {
	listings, err := provider.Fetch(ctx, client, board, since)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, listing := range listings {
		res.Seen++
		metadata, err := json.Marshal(listing.Metadata)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: metadata: %v", listing.CanonicalURL, err))
			continue
		}
		out, err := db.UpsertListingVersion(ctx, conn, db.ListingVersionInput{
			CanonicalURL: canonical(listing.CanonicalURL),
			BoardID:      board.ID,
			Title:        listing.Title,
			Salary:       listing.Salary,
			Body:         listing.BodyHTML,
			FoundAt:      time.Now().UTC(),
			PostedAt:     listing.PostedAt,
			MetadataJSON: string(metadata),
		})
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: upsert: %v", listing.CanonicalURL, err))
			continue
		}
		if out.Inserted {
			res.Inserted++
		} else {
			res.Updated++
		}
	}
	return res, nil
}

func Retry(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for i := 0; i < attempts; i++ {
		if err := fn(); err != nil {
			last = err
		} else {
			return nil
		}
		if i == attempts-1 {
			break
		}
		delay := baseDelay << i
		if delay <= 0 {
			delay = time.Millisecond
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	return last
}

func getJSON(ctx context.Context, client *http.Client, rawURL string, dst any) error {
	return Retry(ctx, 3, 50*time.Millisecond, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "jobfinder job-description-search")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("retryable status %d", resp.StatusCode)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(dst)
	})
}

func getText(ctx context.Context, client *http.Client, rawURL string) (string, error) {
	var body string
	err := Retry(ctx, 3, 50*time.Millisecond, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "jobfinder job-description-search")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("retryable status %d", resp.StatusCode)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		body = string(b)
		return nil
	})
	return body, err
}

func boardEndpoint(boardURL, suffix string) string {
	base := strings.TrimRight(boardURL, "/")
	if strings.HasSuffix(base, suffix) {
		return base
	}
	return base + suffix
}

func canonical(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	u.Fragment = ""
	q := u.Query()
	for key := range q {
		low := strings.ToLower(key)
		if strings.HasPrefix(low, "utm_") || low == "gh_src" || low == "lever-source" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func parseTime(value string) sql.NullTime {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullTime{}
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "2006-01-02T15:04:05.000Z"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return sql.NullTime{Time: t.UTC(), Valid: true}
		}
	}
	return sql.NullTime{}
}

func meta(provider, externalID string) map[string]string {
	m := map[string]string{"provider": provider}
	if externalID != "" {
		m["external_id"] = externalID
	}
	return m
}

// GreenhouseProvider consumes the public job board API and stores full content.
type GreenhouseProvider struct{}

func (GreenhouseProvider) Fetch(ctx context.Context, client *http.Client, board Board, since time.Time) ([]Listing, error) {
	var payload struct {
		Jobs []struct {
			ID             int64  `json:"id"`
			Title          string `json:"title"`
			AbsoluteURL    string `json:"absolute_url"`
			Content        string `json:"content"`
			UpdatedAt      string `json:"updated_at"`
			FirstPublished string `json:"first_published"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, client, boardEndpoint(board.URL, "/jobs?content=true"), &payload); err != nil {
		return nil, err
	}
	out := make([]Listing, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		posted := parseTime(job.FirstPublished)
		if !posted.Valid {
			posted = parseTime(job.UpdatedAt)
		}
		out = append(out, Listing{CanonicalURL: canonical(job.AbsoluteURL), Title: job.Title, BodyHTML: job.Content, PostedAt: posted, Metadata: meta("greenhouse", strconv.FormatInt(job.ID, 10))})
	}
	return out, nil
}

// LeverProvider consumes Lever's postings API with rendered descriptions.
type LeverProvider struct{}

func (LeverProvider) Fetch(ctx context.Context, client *http.Client, board Board, since time.Time) ([]Listing, error) {
	var payload []struct {
		ID          string `json:"id"`
		Text        string `json:"text"`
		HostedURL   string `json:"hostedUrl"`
		Description string `json:"description"`
		CreatedAt   int64  `json:"createdAt"`
		Categories  struct {
			Commitment string `json:"commitment"`
		} `json:"categories"`
	}
	if err := getJSON(ctx, client, boardEndpoint(board.URL, "/postings?mode=json"), &payload); err != nil {
		return nil, err
	}
	out := make([]Listing, 0, len(payload))
	for _, job := range payload {
		posted := sql.NullTime{}
		if job.CreatedAt > 0 {
			posted = sql.NullTime{Time: time.UnixMilli(job.CreatedAt).UTC(), Valid: true}
		}
		out = append(out, Listing{CanonicalURL: canonical(job.HostedURL), Title: job.Text, BodyHTML: job.Description, PostedAt: posted, Metadata: meta("lever", job.ID)})
	}
	return out, nil
}

// AshbyProvider consumes Ashby's public job board API and stores full HTML.
type AshbyProvider struct{}

func (AshbyProvider) Fetch(ctx context.Context, client *http.Client, board Board, since time.Time) ([]Listing, error) {
	var payload struct {
		Jobs []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			JobURL      string `json:"jobUrl"`
			Description string `json:"descriptionHtml"`
			UpdatedAt   string `json:"updatedAt"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, client, boardEndpoint(board.URL, "/jobs"), &payload); err != nil {
		return nil, err
	}
	out := make([]Listing, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		out = append(out, Listing{CanonicalURL: canonical(job.JobURL), Title: job.Title, BodyHTML: job.Description, PostedAt: parseTime(job.UpdatedAt), Metadata: meta("ashby", job.ID)})
	}
	return out, nil
}

// WorkdayProvider stores summaries because many Workday tenants require a detail POST per requisition.
type WorkdayProvider struct{}

func (WorkdayProvider) Fetch(ctx context.Context, client *http.Client, board Board, since time.Time) ([]Listing, error) {
	var payload struct {
		JobPostings []struct {
			Title      string `json:"title"`
			ExternalID string `json:"externalPath"`
			PostedOn   string `json:"postedOn"`
			Summary    string `json:"summary"`
		} `json:"jobPostings"`
	}
	if err := getJSON(ctx, client, boardEndpoint(board.URL, "/jobs"), &payload); err != nil {
		return nil, err
	}
	out := make([]Listing, 0, len(payload.JobPostings))
	for _, job := range payload.JobPostings {
		cu := job.ExternalID
		if strings.HasPrefix(cu, "/") {
			cu = strings.TrimRight(board.URL, "/") + cu
		}
		out = append(out, Listing{CanonicalURL: canonical(cu), Title: job.Title, BodyHTML: job.Summary, PostedAt: parseTime(job.PostedOn), Metadata: meta("workday", job.ExternalID)})
	}
	return out, nil
}

// JobviteProvider consumes listing data embedded by Jobvite board exports.
type JobviteProvider struct{}

func (JobviteProvider) Fetch(ctx context.Context, client *http.Client, board Board, since time.Time) ([]Listing, error) {
	var payload struct {
		Jobs []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Date        string `json:"date"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, client, boardEndpoint(board.URL, "/jobs.json"), &payload); err != nil {
		return nil, err
	}
	out := make([]Listing, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		out = append(out, Listing{CanonicalURL: canonical(job.URL), Title: job.Title, BodyHTML: job.Description, PostedAt: parseTime(job.Date), Metadata: meta("jobvite", job.ID)})
	}
	return out, nil
}

// ClimatebaseProvider sends the freshness cutoff to the server rather than filtering locally.
type ClimatebaseProvider struct{}

func (ClimatebaseProvider) Fetch(ctx context.Context, client *http.Client, board Board, since time.Time) ([]Listing, error) {
	u, err := url.Parse(boardEndpoint(board.URL, "/api/jobs"))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if !since.IsZero() {
		q.Set("posted_after", since.UTC().Format("2006-01-02"))
	}
	u.RawQuery = q.Encode()
	var payload struct {
		Jobs []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			PostedAt    string `json:"posted_at"`
		} `json:"jobs"`
	}
	if err := getJSON(ctx, client, u.String(), &payload); err != nil {
		return nil, err
	}
	out := make([]Listing, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		out = append(out, Listing{CanonicalURL: canonical(job.URL), Title: job.Title, BodyHTML: job.Description, PostedAt: parseTime(job.PostedAt), Metadata: meta("climatebase", job.ID)})
	}
	return out, nil
}

type CommentCache struct {
	conn     *sql.DB
	positive map[string]Listing
	negative map[string]bool
}

func NewCommentCache() *CommentCache {
	return &CommentCache{positive: map[string]Listing{}, negative: map[string]bool{}}
}

func NewDurableCommentCache(ctx context.Context, conn *sql.DB) (*CommentCache, error) {
	if err := ensureCommentCacheTable(ctx, conn); err != nil {
		return nil, err
	}
	return &CommentCache{conn: conn, positive: map[string]Listing{}, negative: map[string]bool{}}, nil
}

func ensureCommentCacheTable(ctx context.Context, conn *sql.DB) error {
	_, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS hn_comment_cache (
		comment_id TEXT PRIMARY KEY,
		is_positive INTEGER NOT NULL CHECK (is_positive IN (0,1)),
		canonical_url TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT '',
		body_html TEXT NOT NULL DEFAULT '',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		last_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`)
	return err
}

func (c *CommentCache) Lookup(ctx context.Context, commentID string) (Listing, bool, bool, error) {
	if c == nil {
		return Listing{}, false, false, nil
	}
	if listing, ok := c.positive[commentID]; ok {
		return listing, true, true, nil
	}
	if c.negative[commentID] {
		return Listing{}, false, true, nil
	}
	if c.conn == nil {
		return Listing{}, false, false, nil
	}
	var isPositive int
	var listing Listing
	var metadataJSON string
	err := c.conn.QueryRowContext(ctx, `SELECT is_positive, canonical_url, title, body_html, metadata_json FROM hn_comment_cache WHERE comment_id = ?`, commentID).Scan(&isPositive, &listing.CanonicalURL, &listing.Title, &listing.BodyHTML, &metadataJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return Listing{}, false, false, nil
	}
	if err != nil {
		return Listing{}, false, false, err
	}
	if _, err := c.conn.ExecContext(ctx, `UPDATE hn_comment_cache SET last_seen_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE comment_id = ?`, commentID); err != nil {
		return Listing{}, false, false, err
	}
	if isPositive == 0 {
		c.negative[commentID] = true
		return Listing{}, false, true, nil
	}
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &listing.Metadata); err != nil {
			return Listing{}, false, false, err
		}
	}
	c.positive[commentID] = listing
	return listing, true, true, nil
}

func (c *CommentCache) StorePositive(ctx context.Context, commentID string, listing Listing) error {
	if c == nil {
		return nil
	}
	c.positive[commentID] = listing
	delete(c.negative, commentID)
	if c.conn == nil {
		return nil
	}
	metadata, err := json.Marshal(listing.Metadata)
	if err != nil {
		return err
	}
	_, err = c.conn.ExecContext(ctx, `INSERT INTO hn_comment_cache (comment_id, is_positive, canonical_url, title, body_html, metadata_json)
		VALUES (?, 1, ?, ?, ?, ?)
		ON CONFLICT(comment_id) DO UPDATE SET
			is_positive = 1,
			canonical_url = excluded.canonical_url,
			title = excluded.title,
			body_html = excluded.body_html,
			metadata_json = excluded.metadata_json,
			last_seen_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`, commentID, listing.CanonicalURL, listing.Title, listing.BodyHTML, string(metadata))
	return err
}

func (c *CommentCache) StoreNegative(ctx context.Context, commentID string) error {
	if c == nil {
		return nil
	}
	c.negative[commentID] = true
	delete(c.positive, commentID)
	if c.conn == nil {
		return nil
	}
	_, err := c.conn.ExecContext(ctx, `INSERT INTO hn_comment_cache (comment_id, is_positive)
		VALUES (?, 0)
		ON CONFLICT(comment_id) DO UPDATE SET
			is_positive = 0,
			canonical_url = '',
			title = '',
			body_html = '',
			metadata_json = '{}',
			last_seen_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')`, commentID)
	return err
}

// HackerNewsProvider scrapes Who's Hiring comments and caches both matches and misses by comment id.
type HackerNewsProvider struct{ Cache *CommentCache }

func (p HackerNewsProvider) Fetch(ctx context.Context, client *http.Client, board Board, since time.Time) ([]Listing, error) {
	if p.Cache == nil {
		p.Cache = NewCommentCache()
	}
	body, err := getText(ctx, client, board.URL)
	if err != nil {
		return nil, err
	}
	comments := parseHNComments(body)
	out := make([]Listing, 0, len(comments))
	for _, comment := range comments {
		cached, positive, found, err := p.Cache.Lookup(ctx, comment.ID)
		if err != nil {
			return nil, err
		}
		if found {
			if positive {
				out = append(out, cached)
			}
			continue
		}
		listing, ok := listingFromHNComment(comment)
		if !ok {
			if err := p.Cache.StoreNegative(ctx, comment.ID); err != nil {
				return nil, err
			}
			continue
		}
		if err := p.Cache.StorePositive(ctx, comment.ID, listing); err != nil {
			return nil, err
		}
		out = append(out, listing)
	}
	return out, nil
}

type hnComment struct{ ID, HTML string }

var commentRE = regexp.MustCompile(`(?s)<tr class=['"]athing comtr['"] id=['"]([^'"]+)['"].*?<span class=['"]commtext[^'"]*['"]>(.*?)</span>`)
var hrefRE = regexp.MustCompile(`href=['"]([^'"]+)['"]`)

func parseHNComments(page string) []hnComment {
	matches := commentRE.FindAllStringSubmatch(page, -1)
	out := make([]hnComment, 0, len(matches))
	for _, m := range matches {
		out = append(out, hnComment{ID: m[1], HTML: m[2]})
	}
	return out
}

func listingFromHNComment(comment hnComment) (Listing, bool) {
	text := strings.ToLower(stripTags(comment.HTML))
	if !(strings.Contains(text, "hiring") || strings.Contains(text, "remote") || strings.Contains(text, "onsite")) {
		return Listing{}, false
	}
	link := "https://news.ycombinator.com/item?id=" + url.QueryEscape(comment.ID)
	if m := hrefRE.FindStringSubmatch(comment.HTML); len(m) == 2 && strings.HasPrefix(m[1], "http") {
		link = m[1]
	}
	title := strings.TrimSpace(stripTags(comment.HTML))
	if title == "" {
		return Listing{}, false
	}
	if len(title) > 120 {
		title = title[:120]
	}
	return Listing{CanonicalURL: canonical(link), Title: title, BodyHTML: comment.HTML, Metadata: meta("hackernews", comment.ID)}, true
}

func stripTags(s string) string {
	return strings.TrimSpace(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, " "))
}

func ActiveBoards(ctx context.Context, conn *sql.DB) ([]Board, error) {
	rows, err := conn.QueryContext(ctx, `SELECT b.board_id, b.board_url, COALESCE(c.name,b.company), coalesce(b.ats_type,''), b.metadata_json FROM job_boards b LEFT JOIN companies c ON c.company_id=b.company_id WHERE b.status='active' ORDER BY b.board_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.URL, &b.Company, &b.ATSType, &b.Metadata); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func Run(ctx context.Context, conn *sql.DB, client *http.Client, since time.Time) (Result, error) {
	boards, err := ActiveBoards(ctx, conn)
	if err != nil {
		return Result{}, err
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	hnCache, err := NewDurableCommentCache(ctx, conn)
	if err != nil {
		return Result{}, err
	}
	var total Result
	for _, board := range boards {
		provider, ok := ProviderFor(board.ATSType)
		if !ok {
			total.Errors = append(total.Errors, fmt.Sprintf("board %d unsupported ats_type %q", board.ID, board.ATSType))
			continue
		}
		if isHackerNews(board.ATSType) {
			provider = HackerNewsProvider{Cache: hnCache}
		}
		res, err := IngestBoard(ctx, conn, client, board, since, provider)
		if err != nil {
			total.Errors = append(total.Errors, fmt.Sprintf("board %d fetch: %v", board.ID, err))
			continue
		}
		total.Seen += res.Seen
		total.Inserted += res.Inserted
		total.Updated += res.Updated
		total.Errors = append(total.Errors, res.Errors...)
	}
	sort.Strings(total.Errors)
	if len(total.Errors) > 0 {
		return total, errors.New("job description search completed with errors")
	}
	return total, nil
}
