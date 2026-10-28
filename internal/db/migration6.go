package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var migration6 = []string{
	`ALTER TABLE companies ADD COLUMN normalized_name TEXT`,
	`UPDATE companies SET normalized_name=lower(trim(name))`,
	`CREATE UNIQUE INDEX idx_companies_normalized_name ON companies(normalized_name)`,
	`ALTER TABLE companies ADD COLUMN metadata_json TEXT NOT NULL DEFAULT '[]'`,
	`CREATE TABLE company_aliases (
		alias_id INTEGER PRIMARY KEY,
		company_id INTEGER NOT NULL REFERENCES companies(company_id) ON DELETE CASCADE,
		alias TEXT NOT NULL COLLATE NOCASE UNIQUE,
		source TEXT NOT NULL DEFAULT 'manual',
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
	)`,
	`CREATE INDEX idx_company_aliases_company ON company_aliases(company_id)`,
	`CREATE TABLE company_job_sites (
		site_id INTEGER PRIMARY KEY,
		company_id INTEGER NOT NULL REFERENCES companies(company_id) ON DELETE CASCADE,
		site_url TEXT NOT NULL,
		official_site TEXT,
		ats_type TEXT,
		identity_status TEXT,
		organization_type TEXT,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		discovered_at TEXT,
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
		UNIQUE(company_id,site_url)
	)`,
	`CREATE INDEX idx_company_job_sites_company ON company_job_sites(company_id)`,
	`ALTER TABLE job_boards ADD COLUMN company_id INTEGER REFERENCES companies(company_id) ON DELETE RESTRICT`,
	`ALTER TABLE job_boards ADD COLUMN site_id INTEGER REFERENCES company_job_sites(site_id) ON DELETE SET NULL`,
	`UPDATE job_boards SET site_id=(SELECT s.site_id FROM company_job_sites s WHERE s.company_id=job_boards.company_id AND s.site_url=job_boards.board_url)`,
	`CREATE INDEX idx_job_boards_company ON job_boards(company_id)`,
	`CREATE INDEX idx_job_boards_site ON job_boards(site_id)`,
	`INSERT INTO schema_migrations(version) VALUES (6)`,
}

func applyMigration6(ctx context.Context, tx *sql.Tx) error {
	if err := execAll(ctx, tx, migration6); err != nil {
		return err
	}
	recovery, err := loadRootRecoveries(rootRecoveryDefaultPath)
	if err != nil {
		return err
	}
	return migrateCompanyCareerResearch(ctx, tx, recovery)
}

// RootRecovery is one record from scripts/url_root_recovery.py output.
type rootRecovery struct {
	RecoveredURL string
	CandidateURL string
	Tier         string
}

// rootRecoveryDefaultPath is resolved relative to the process working
// directory. The migration requires this file to exist for production deployments
// to prevent irreversible data loss from dropping company_careers without
// applying recovery data.
var rootRecoveryDefaultPath = filepath.Join("artifacts", "url-root-recovery", "results.jsonl")

func loadRootRecoveries(path string) (map[string]rootRecovery, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// In production, this is a hard requirement - fail the migration if missing
		if os.IsNotExist(err) && isProduction() {
			return nil, fmt.Errorf("migration 6 requires recovery file at %s for production deployments: %w", path, err)
		}
		return nil, nil
	}
	out := map[string]rootRecovery{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			SourceID     string `json:"source_id"`
			RecoveredURL string `json:"recovered_url"`
			CandidateURL string `json:"candidate_url"`
			Tier         string `json:"tier"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil || rec.SourceID == "" {
			continue
		}
		out[rec.SourceID] = rootRecovery{RecoveredURL: rec.RecoveredURL, CandidateURL: rec.CandidateURL, Tier: rec.Tier}
	}
	return out, nil
}

// isProduction returns true if running in a production environment
func isProduction() bool {
	return os.Getenv("ENVIRONMENT") == "production" || os.Getenv("GO_ENV") == "production"
}

var (
	careersDetailRE  = regexp.MustCompile(`(?i)(/Details/|/details/\d|/job/|/jobs/\d{4,}|/position/|/posting/|/opportunit(?:y|ies)/\d|/requisition|/opening/|/careers/job|/jobs/view|/en/job)`)
	notEmployerHosts = map[string]bool{
		"en.wikipedia.org": true, "www.wikipedia.org": true, "www.google.com": true,
		"www.linkedin.com": true, "www.indeed.com": true, "www.glassdoor.com": true,
		"www.greatplacetowork.com": true, "www.ziprecruiter.com": true,
	}
	multiTenantHosts = map[string]bool{
		"jobs.ashbyhq.com": true, "recruiting.paylocity.com": true,
		"apply.workable.com": true, "jobs.gem.com": true, "jobs.workable.com": true,
		"jobs.lever.co": true, "boards.greenhouse.io": true,
		"job-boards.greenhouse.io": true, "jobs.jobvite.com": true,
		"jobs.smartrecruiters.com": true,
	}
)

// classifyCareersURL mirrors scripts/url_root_recovery.py classification.
func classifyCareersURL(rawURL string, hostPathCounts map[string]int) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unparseable"
	}
	host, path := strings.ToLower(u.Host), u.Path
	if notEmployerHosts[host] {
		return "non-employer"
	}
	if careersDetailRE.MatchString(path) {
		return "job-detail"
	}
	if path == "" || path == "/" {
		if multiTenantHosts[host] || hostPathCounts[host] > 1 {
			return "bare-shared-host"
		}
		return "bare-company-host"
	}
	return "ok"
}

// migrateCompanyCareerResearch imports all remaining research records before
// removing the staging table. Company identity uses exact normalized names;
// shared URLs never merge companies. Site rows are created only for clean or
// recovered careers URLs; problematic URLs without a verified root become
// evidence-only (company row kept, no site row). Multiple evidence rows for
// one company/site are retained as a JSON array in metadata_json.
func migrateCompanyCareerResearch(ctx context.Context, tx *sql.Tx, recovery map[string]rootRecovery) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='company_careers'`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return linkExistingBoards(ctx, tx)
	}
	rows, err := tx.QueryContext(ctx, `SELECT source_id,observed_name,canonical_name,careers_page,official_site,ats,organization_type,identity_status,reason_code,search_attempts,route,researched_at FROM company_careers ORDER BY source_id`)
	if err != nil {
		return err
	}
	type record struct {
		id, observed                                                         string
		canonical, page, official, ats, org, status, reason, route, research sql.NullString
		attempts                                                             int
	}
	var all []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.id, &r.observed, &r.canonical, &r.page, &r.official, &r.ats, &r.org, &r.status, &r.reason, &r.attempts, &r.route, &r.research); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	hostPathCounts := map[string]int{}
	for _, r := range all {
		if u, err := url.Parse(strings.TrimSpace(r.page.String)); err == nil && u.Path != "" && u.Path != "/" {
			hostPathCounts[strings.ToLower(u.Host)]++
		}
	}
	for _, r := range all {
		name := strings.TrimSpace(r.canonical.String)
		if name == "" {
			name = strings.TrimSpace(r.observed)
		}
		page := strings.TrimSpace(r.page.String)
		if name == "" || page == "" {
			return fmt.Errorf("research row %s has blank company name or career page", r.id)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO companies(name,normalized_name) VALUES (?,lower(trim(?))) ON CONFLICT(normalized_name) DO NOTHING`, name, name); err != nil {
			return err
		}
		var companyID int64
		if err := tx.QueryRowContext(ctx, `SELECT company_id FROM companies WHERE normalized_name=lower(trim(?))`, name).Scan(&companyID); err != nil {
			return err
		}
		observed := strings.TrimSpace(r.observed)
		if observed != "" && !strings.EqualFold(name, observed) {
			var other int64
			err := tx.QueryRowContext(ctx, `SELECT company_id FROM companies WHERE normalized_name=lower(trim(?))`, observed).Scan(&other)
			if err == sql.ErrNoRows {
				_, err = tx.ExecContext(ctx, `INSERT INTO company_aliases(company_id,alias,source) VALUES (?,?,'company_careers') ON CONFLICT(alias) DO NOTHING`, companyID, observed)
			}
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			// If observed name is already a distinct company, preserve it as a company,
			// not as an alias of the canonical name.
		}
		evidenceAny := map[string]any{"source_id": r.id, "reason_code": r.reason.String, "route": r.route.String, "search_attempts": r.attempts, "researched_at": r.research.String}
		siteURL := ""
		category := classifyCareersURL(page, hostPathCounts)
		if category == "ok" || category == "bare-company-host" {
			siteURL = page
		} else if rec, ok := recovery[r.id]; ok {
			switch {
			case rec.Tier == "verified" && rec.RecoveredURL != "":
				siteURL = rec.RecoveredURL
				evidenceAny["recovery"] = map[string]string{"tier": "recovered-verified", "original_careers_page": page, "category": category}
			case rec.Tier == "unverified-candidate" && rec.CandidateURL != "":
				siteURL = rec.CandidateURL
				evidenceAny["recovery"] = map[string]string{"tier": "recovered-unverified", "original_careers_page": page, "category": category}
			}
		}
		evidence, _ := json.Marshal(evidenceAny)
		
		if siteURL == "" {
			// Evidence-only: preserve research metadata in company metadata_json
			// even though we don't create a site row
			evidenceMeta, _ := json.Marshal(map[string]any{
				"evidence_only": true,
				"careers_page": page,
				"category": category,
				"evidence": evidenceAny,
			})
			_, err = tx.ExecContext(ctx, `UPDATE companies SET metadata_json=json_insert(COALESCE(metadata_json,'[]'),'$[#]',json(?)) WHERE company_id=?`, string(evidenceMeta), companyID)
			if err != nil {
				return fmt.Errorf("migrate evidence-only row %s: %w", r.id, err)
			}
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO company_job_sites(company_id,site_url,official_site,ats_type,identity_status,organization_type,metadata_json,discovered_at)
			VALUES(?,?,?,?,?,?,json_array(json(?)),?) ON CONFLICT(company_id,site_url) DO UPDATE SET
			official_site=COALESCE(excluded.official_site,company_job_sites.official_site),
			ats_type=COALESCE(excluded.ats_type,company_job_sites.ats_type),
			identity_status=COALESCE(excluded.identity_status,company_job_sites.identity_status),
			organization_type=COALESCE(excluded.organization_type,company_job_sites.organization_type),
			metadata_json=json_insert(company_job_sites.metadata_json,'$[#]',json_extract(excluded.metadata_json,'$[0]'))`, companyID, siteURL, nullableString(r.official), nullableString(r.ats), nullableString(r.status), nullableString(r.org), string(evidence), nullableString(r.research))
		if err != nil {
			return fmt.Errorf("migrate research row %s: %w", r.id, err)
		}
	}
	if err := linkExistingBoards(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DROP TABLE company_careers`)
	return err
}

func linkExistingBoards(ctx context.Context, tx *sql.Tx) error {
	// Create company rows for any historical board names, without merging names.
	if _, err := tx.ExecContext(ctx, `INSERT INTO companies(name,normalized_name) SELECT DISTINCT trim(company),lower(trim(company)) FROM job_boards WHERE trim(company)<>'' ON CONFLICT(normalized_name) DO NOTHING`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE job_boards SET company_id=(SELECT company_id FROM companies WHERE normalized_name=lower(trim(job_boards.company)))`); err != nil {
		return err
	}
	var unlinked int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM job_boards WHERE company_id IS NULL`).Scan(&unlinked); err != nil {
		return err
	}
	if unlinked > 0 {
		return fmt.Errorf("%d job boards could not be linked to a company", unlinked)
	}
	return nil
}

func nullableString(s sql.NullString) any {
	if !s.Valid || strings.TrimSpace(s.String) == "" {
		return nil
	}
	return s.String
}
