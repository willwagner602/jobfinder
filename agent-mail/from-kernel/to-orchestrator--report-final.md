# Shared database kernel final report

## Round-2 migration safety fix

Fixed the critical migration-safety finding:

- `internal/db.EnsureCurrent` now pins migration work to one SQLite connection so `PRAGMA foreign_keys` changes apply to the same connection used for migration and validation.
- Upgrade path enables foreign keys for version inspection, disables enforcement only around the schema rebuild, runs all migrations in a transaction, runs `PRAGMA foreign_key_check` before commit, commits only if clean, and restores foreign-key enforcement afterwards.
- Migration 2 still rebuilds `job_listing_versions` to remove the old `UNIQUE (job_listing_id, content_hash)` constraint needed for A → B → A history.
- The v1 upgrade fixture now includes a board, user, listing, listing version, and match row referencing that listing version.
- Upgrade regression coverage now asserts:
  - upgrade succeeds from the v1 fixture with dependent match data
  - schema reaches `CurrentVersion`
  - preserved listing version `found_at`, full text/body, and content hash
  - preserved match rating/decision/data
  - `PRAGMA foreign_key_check` returns no violations after migration
  - upgraded schema accepts a repeated historical content hash for a later version

The prior A → B → A behavior fix and regression test remain in place.

## Round-1 review fix retained

- `internal/db.UpsertListingVersion` compares incoming content hash only against the current/latest listing version.
- If the incoming hash matches the latest version, it updates listing metadata/last-seen fields and returns the current version without inserting.
- If the incoming hash differs from the latest version, it appends `current_version + 1`, even when that hash appeared in an older version.
- Regression coverage still expects versions 1, 2, and 3 for an A → B → A sequence with preserved `found_at`, normalized full text, and salary records.

## Built

- Created Go module `jobs.willwagner.me` with the shared database kernel under owned paths only.
- Added `internal/schema` contracts:
  - `CurrentVersion = 2`
  - shared status types for process history, boards, users, and match decisions
  - `NormalizeBodyText` and `ListingContentHash(title, salary, body)` for canonical listing version decisions
- Added `internal/db` SQLite kernel:
  - `Open(path)` with foreign keys enabled
  - `EnsureCurrent(ctx, *sql.DB)` schema initialization/upgrade path
  - `Version(ctx, *sql.DB)` version reader
  - versioned migrations with `schema_migrations`
  - shared tables for users/auth, user config/history, job boards, canonical job listings, immutable listing versions, per-user matches, and shared process history
  - `UpsertListingVersion` helper that uses canonical URL identity, updates listing metadata independently, and inserts immutable version rows when title + salary + normalized body hash differs from the latest version
- Added prior-version fixture at `internal/db/testdata/schema_v1.sql` for upgrade tests.
- Added minimal orchestrator startup contract at `cmd/orchestrator.Startup(ctx, conn)` that upgrades the database before services run.
- Added database verification documentation at `docs/database/verification.md`, including dependency rationale for `github.com/mattn/go-sqlite3`.

## Contract surface for leaf tasks

Leaf services can depend on:

- `internal/schema.CurrentVersion`
- `internal/schema` status constants:
  - process history: `success`, `failure`, `partial`
  - boards: `active`, `unsupported`
  - users: `pending`, `live`, `suspended`
  - matches: `match`, `no_match`, `failure`, `deferred`
- `internal/schema.NormalizeBodyText` and `internal/schema.ListingContentHash`
- `internal/db.Open`, `internal/db.EnsureCurrent`, `internal/db.Version`
- `internal/db.UpsertListingVersion` with `ListingVersionInput` / `ListingVersionResult`
- Database table contracts:
  - `users`
  - `user_configs`
  - `user_history`
  - `job_boards`
  - `job_listings`
  - `job_listing_versions`
  - `job_matches`
  - `job_history`

## Tests/checks

Available checks run for the round-2 fix:

```sh
git diff --check
```

Result: passed.

```sh
python3 - <<'PY'
# Loaded internal/db/testdata/schema_v1.sql into sqlite3 with foreign_keys=ON,
# disabled enforcement for the migration rebuild, recreated job_listing_versions
# without UNIQUE(job_listing_id, content_hash), copied rows, dropped/renamed,
# recreated indexes, inserted schema version 2, ran PRAGMA foreign_key_check,
# committed, verified preserved version and match rows, and inserted a repeated
# historical content hash for version 2.
PY
```

Result: passed (`python sqlite migration reproduction passed`).

Intended Go verification command remains:

```sh
gofmt -w internal/schema/schema.go internal/schema/schema_test.go internal/db/db.go internal/db/db_test.go cmd/orchestrator/orchestrator.go cmd/orchestrator/orchestrator_test.go && go mod tidy && go test ./...
```

Result in this worktree: **blocked by missing Go toolchain**. The original attempt failed immediately with:

```text
/usr/bin/bash: line 61: gofmt: command not found
```

Follow-up checks still show neither `go` nor `gofmt` is on `PATH`. Because the toolchain is unavailable, I could not generate `go.sum`, run `gofmt`, `go mod tidy`, or `go test ./...` locally. The committed tests are intended to cover fresh initialization, previous schema upgrade, schema version consistency, listing uniqueness/immutability, normalized content hashing, A → B → A content reversion, migration preservation of dependent match rows, `PRAGMA foreign_key_check`, and key/check constraints.

## Commits

- Initial kernel commit: `6fd5f182e794beb14a609bddafbe86b16810b62a`
- Round-1 fix commit: `4dd9efe38dc012595e4db8b6e3c7d2285bd36664`
- Round-2 fix commit: `4e2abe5e13f17ab1ba1b2757a0e945305aef55fd`
- Branch: `kernel`
- Working tree: clean after commit

## Assumptions and unresolved issues

- Chose `github.com/mattn/go-sqlite3` because Go's standard library has `database/sql` but no SQLite driver.
- Schema version `1` is the prior fixture; current schema version is `2`, with the upgrade adding indexes and rebuilding `job_listing_versions` to allow repeated historical content hashes across different version numbers while preserving existing matches.
- No leaf-service scraping, matching, auth UI, or admin behavior was implemented beyond shared contracts and startup upgrade wiring.
- Unresolved verification risk: the code needs to be formatted/tidied/tested in an environment with Go installed.
