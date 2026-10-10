package boards

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"jobs.willwagner.me/internal/schema"
)

const (
	ServiceName          = "job-board-search"
	DefaultAdzunaMaxCalls = 100
	DefaultAdzunaWindowDays = 8
	DefaultSerpQueries   = 3
	DefaultSerpPages     = 2
)

// Candidate is the provider-neutral import boundary for discovered boards.
// Candidates are persisted as discovered or unsupported boards, but are not used to
// activate downstream scraping work by this service.
type Candidate struct {
	Provider        string            `json:"provider"`
	Identifier      string            `json:"identifier"`
	URL             string            `json:"url"`
	Company         string            `json:"company"`
	ATSType         string            `json:"ats_type,omitempty"`
	Supported       bool              `json:"supported"`
	Provenance      string            `json:"provenance,omitempty"`
	DiscoveredAt    time.Time         `json:"discovered_at"`
	UpstreamStatus  string            `json:"upstream_status,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Board is the durable, source-neutral board shape synchronized with SQLite
// and data/job-boards.json.
type Board struct {
	URL          string            `json:"url"`
	Company      string            `json:"company"`
	ATSType      string            `json:"ats_type,omitempty"`
	Status       schema.BoardStatus `json:"status"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	DiscoveredAt time.Time         `json:"discovered_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type Provider interface {
	Name() string
	Discover(context.Context) ([]Candidate, error)
}

type Config struct {
	JSONPath string
	Now      func() time.Time
}

type Result struct {
	ProviderResults []ProviderResult
	Candidates      int
	Inserted        int
	Updated         int
	JSONBoards      int
}

type ProviderResult struct {
	Provider   string `json:"provider"`
	Candidates int    `json:"candidates"`
	Error      string `json:"error,omitempty"`
}

type storeResult struct { inserted, updated int }

func Run(ctx context.Context, conn *sql.DB, providers []Provider, cfg Config) (Result, error) {
	if conn == nil { return Result{}, errors.New("database connection is required") }
	if cfg.Now == nil { cfg.Now = func() time.Time { return time.Now().UTC() } }
	if cfg.JSONPath == "" { cfg.JSONPath = "data/job-boards.json" }
	started := cfg.Now()
	res := Result{}
	type discovery struct {
		result     ProviderResult
		candidates []Candidate
	}
	ch := make(chan discovery)
	var wg sync.WaitGroup
	startedProviders := 0
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		startedProviders++
		wg.Add(1)
		go func(provider Provider) {
			defer wg.Done()
			candidates, err := provider.Discover(ctx)
			pr := ProviderResult{Provider: provider.Name(), Candidates: len(candidates)}
			if err != nil {
				pr.Error = err.Error()
			}
			ch <- discovery{result: pr, candidates: candidates}
		}(provider)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	var all []Candidate
	var errs []string
	for item := range ch {
		res.ProviderResults = append(res.ProviderResults, item.result)
		all = append(all, item.candidates...)
		if item.result.Error != "" {
			errs = append(errs, item.result.Provider+": "+item.result.Error)
		}
	}
	if startedProviders == 0 {
		return res, errors.New("at least one provider is required")
	}
	sort.Slice(res.ProviderResults, func(i, j int) bool { return res.ProviderResults[i].Provider < res.ProviderResults[j].Provider })
	res.Candidates = len(all)
	stored, err := upsertCandidates(ctx, conn, all, cfg.Now)
	if err != nil {
		_ = recordHistory(ctx, conn, started, cfg.Now(), schema.ProcessStatusFailure, res.Candidates, res.ProviderResults, err.Error())
		return res, err
	}
	res.Inserted, res.Updated = stored.inserted, stored.updated
	boards, err := loadBoards(ctx, conn)
	if err != nil {
		_ = recordHistory(ctx, conn, started, cfg.Now(), schema.ProcessStatusFailure, res.Candidates, res.ProviderResults, err.Error())
		return res, err
	}
	if err := WriteJSON(cfg.JSONPath, boards); err != nil {
		_ = recordHistory(ctx, conn, started, cfg.Now(), schema.ProcessStatusFailure, res.Candidates, res.ProviderResults, err.Error())
		return res, err
	}
	res.JSONBoards = len(boards)
	status := schema.ProcessStatusSuccess
	var errBlob string
	if len(errs) > 0 { status = schema.ProcessStatusPartial; errBlob = strings.Join(errs, "; ") }
	if err := recordHistory(ctx, conn, started, cfg.Now(), status, res.Candidates, res.ProviderResults, errBlob); err != nil { return res, err }
	if len(errs) > 0 { return res, fmt.Errorf("provider errors: %s", errBlob) }
	return res, nil
}

func upsertCandidates(ctx context.Context, conn *sql.DB, candidates []Candidate, now func() time.Time) (storeResult, error) {
	var res storeResult
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil { return res, err }
	defer tx.Rollback()
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		board, err := candidate.Board(now)
		if err != nil { return res, err }
		if _, ok := seen[board.URL]; ok { continue }
		seen[board.URL] = struct{}{}
		metadata, err := json.Marshal(board.Metadata)
		if err != nil { return res, err }
		var existing int64
		err = tx.QueryRowContext(ctx, `SELECT board_id FROM job_boards WHERE board_url = ?`, board.URL).Scan(&existing)
		if err == sql.ErrNoRows {
			_, err = tx.ExecContext(ctx, `INSERT INTO job_boards (board_url, company, ats_type, status, metadata_json, discovered_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, board.URL, board.Company, nullString(board.ATSType), board.Status, string(metadata), board.DiscoveredAt.Format(time.RFC3339Nano), board.UpdatedAt.Format(time.RFC3339Nano))
			if err != nil { return res, err }
			res.inserted++
			continue
		}
		if err != nil { return res, err }
		_, err = tx.ExecContext(ctx, `UPDATE job_boards SET company = ?, ats_type = ?, status = CASE WHEN status = 'active' THEN status ELSE ? END, metadata_json = ?, updated_at = ? WHERE board_id = ?`, board.Company, nullString(board.ATSType), board.Status, string(metadata), board.UpdatedAt.Format(time.RFC3339Nano), existing)
		if err != nil { return res, err }
		res.updated++
	}
	return res, tx.Commit()
}

func (c Candidate) Board(now func() time.Time) (Board, error) {
	canonical, err := canonicalURL(c.URL)
	if err != nil { return Board{}, err }
	company := strings.TrimSpace(c.Company)
	if company == "" { company = strings.TrimSpace(c.Identifier) }
	if company == "" { return Board{}, errors.New("candidate company or identifier is required") }
	provider := strings.TrimSpace(c.Provider)
	if provider == "" { return Board{}, errors.New("candidate provider is required") }
	discovered := c.DiscoveredAt.UTC()
	if discovered.IsZero() { discovered = now() }
	status := schema.BoardStatusUnsupported
	if c.Supported { status = schema.BoardStatusDiscovered }
	metadata := map[string]string{"provider": provider}
	if c.Identifier != "" { metadata["identifier"] = c.Identifier }
	if c.Provenance != "" { metadata["provenance"] = c.Provenance }
	if c.UpstreamStatus != "" { metadata["upstream_status"] = c.UpstreamStatus }
	for k, v := range c.Metadata { if strings.TrimSpace(k) != "" { metadata[k] = v } }
	return Board{URL: canonical, Company: company, ATSType: strings.TrimSpace(c.ATSType), Status: status, Metadata: metadata, DiscoveredAt: discovered, UpdatedAt: now()}, nil
}

func canonicalURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" { return "", errors.New("candidate url is required") }
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" { return "", fmt.Errorf("invalid board url %q", raw) }
	u.Fragment = ""
	u.RawQuery = ""
	u.Host = strings.ToLower(u.Host)
	if u.Path != "/" { u.Path = strings.TrimRight(u.EscapedPath(), "/") }
	return u.String(), nil
}

func loadBoards(ctx context.Context, conn *sql.DB) ([]Board, error) {
	rows, err := conn.QueryContext(ctx, `SELECT board_url, company, coalesce(ats_type, ''), status, metadata_json, discovered_at, updated_at FROM job_boards ORDER BY company, board_url`)
	if err != nil { return nil, err }
	defer rows.Close()
	var boards []Board
	for rows.Next() {
		var b Board
		var metadata string
		var discovered, updated string
		if err := rows.Scan(&b.URL, &b.Company, &b.ATSType, &b.Status, &metadata, &discovered, &updated); err != nil { return nil, err }
		if metadata != "" { _ = json.Unmarshal([]byte(metadata), &b.Metadata) }
		b.DiscoveredAt, _ = time.Parse(time.RFC3339Nano, discovered)
		b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		boards = append(boards, b)
	}
	return boards, rows.Err()
}

func WriteJSON(path string, boards []Board) error {
	sort.Slice(boards, func(i, j int) bool { if boards[i].Company == boards[j].Company { return boards[i].URL < boards[j].URL }; return boards[i].Company < boards[j].Company })
	b, err := json.MarshalIndent(boards, "", "  ")
	if err != nil { return err }
	b = append(b, '\n')
	if err := os.MkdirAll(dir(path), 0o755); err != nil { return err }
	return os.WriteFile(path, b, 0o644)
}

func recordHistory(ctx context.Context, conn *sql.DB, started, finished time.Time, status schema.ProcessStatus, items int, providerResults []ProviderResult, errText string) error {
	data, err := json.Marshal(map[string]any{"providers": providerResults})
	if err != nil { return err }
	var errJSON any
	if errText != "" { b, _ := json.Marshal(map[string]string{"error": errText}); errJSON = string(b) }
	_, err = conn.ExecContext(ctx, `INSERT INTO job_history (service, started_at, finished_at, status, items_processed, data_json, error_json) VALUES (?, ?, ?, ?, ?, ?, ?)`, ServiceName, started.UTC().Format(time.RFC3339Nano), finished.UTC().Format(time.RFC3339Nano), status, items, string(data), errJSON)
	return err
}

func nullString(s string) any { if strings.TrimSpace(s) == "" { return nil }; return strings.TrimSpace(s) }

func dir(path string) string { i := strings.LastIndex(path, "/"); if i < 0 { return "." }; if i == 0 { return "/" }; return path[:i] }
