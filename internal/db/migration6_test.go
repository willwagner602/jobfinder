package db

import (
	"context"
	"testing"
)

const researchFixtureSchema = `CREATE TABLE company_careers(source_id TEXT PRIMARY KEY, observed_name TEXT NOT NULL, canonical_name TEXT, identity_status TEXT NOT NULL, careers_page TEXT, official_site TEXT, ats TEXT, organization_type TEXT, reason_code TEXT, search_attempts INTEGER NOT NULL DEFAULT 0, route TEXT, researched_at TEXT)`

func TestMigration6ImportsCareerResearchAndLinksBoards(t *testing.T) {
	conn := openCurrentTestDB(t)
	ctx := context.Background()
	if _, err := conn.Exec(researchFixtureSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO company_careers(source_id,observed_name,canonical_name,identity_status,careers_page,official_site,ats,organization_type,reason_code,search_attempts,route,researched_at) VALUES
	('1','Acme, Inc.','Acme','verified-official-careers','https://jobs.example/acme','https://acme.example','greenhouse','employer','found',2,'review','2025-01-01'),
	('2','Acme','Acme','verified-official-careers','https://jobs.example/acme','https://acme.example','greenhouse','employer','confirmed',3,'done','2025-01-02'),
	('3','Other LLC',NULL,'verified-official-identity','https://jobs.example','https://other.example',NULL,'employer','evidence',1,'review','2025-01-03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO job_boards(board_url,company,ats_type,status) VALUES('https://jobs.example/acme','Acme','greenhouse','active')`); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateCompanyCareerResearch(ctx, tx, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var companies, sites, aliases, source int
	for _, q := range []struct {
		sql  string
		dest *int
	}{{`SELECT count(*) FROM companies WHERE normalized_name='acme'`, &companies}, {`SELECT count(*) FROM company_job_sites`, &sites}, {`SELECT count(*) FROM company_aliases`, &aliases}, {`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='company_careers'`, &source}} {
		if err := conn.QueryRow(q.sql).Scan(q.dest); err != nil {
			t.Fatal(err)
		}
	}
	if companies != 1 || sites != 1 || aliases != 1 || source != 0 {
		t.Fatalf("companies=%d sites=%d aliases=%d source=%d", companies, sites, aliases, source)
	}
	var totalCompanies, otherSites int
	if err := conn.QueryRow(`SELECT count(*) FROM companies`).Scan(&totalCompanies); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM company_job_sites s JOIN companies c ON c.company_id=s.company_id WHERE c.normalized_name='other llc'`).Scan(&otherSites); err != nil {
		t.Fatal(err)
	}
	if totalCompanies != 2 || otherSites != 0 {
		t.Fatalf("total_companies=%d other_llc_sites=%d; evidence-only row should create company but no site", totalCompanies, otherSites)
	}
	var evidenceRows int
	if err := conn.QueryRow(`SELECT json_array_length(metadata_json) FROM company_job_sites WHERE site_url='https://jobs.example/acme'`).Scan(&evidenceRows); err != nil {
		t.Fatal(err)
	}
	if evidenceRows != 2 {
		t.Fatalf("evidence records=%d, want 2", evidenceRows)
	}
	var boardCompanyID, companyID int64
	if err := conn.QueryRow(`SELECT company_id FROM job_boards WHERE board_url='https://jobs.example/acme'`).Scan(&boardCompanyID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT company_id FROM companies WHERE normalized_name='acme'`).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	if boardCompanyID != companyID {
		t.Fatalf("board company id %d != %d", boardCompanyID, companyID)
	}
	if err := foreignKeyCheck(ctx, conn); err != nil {
		t.Fatal(err)
	}
}

func TestMigration6RetainsResearchOnInvalidRow(t *testing.T) {
	conn := openCurrentTestDB(t)
	ctx := context.Background()
	if _, err := conn.Exec(researchFixtureSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO company_careers(source_id,observed_name,identity_status) VALUES('bad','No page','candidate')`); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateCompanyCareerResearch(ctx, tx, nil); err == nil {
		t.Fatal("expected invalid research row failure")
	}
	_ = tx.Rollback()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM company_careers`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("source row count %d", n)
	}
}
