# Kernel review r1 — schema v3 job_boards discovered status

Verdict: clean

## Scope reviewed

- Commit reviewed: `5bf7ee8 Add discovered job board status migration` against parent `4e2abe5e13f17ab1ba1b2757a0e945305aef55fd`
- Owned files reviewed in full:
  - `internal/schema/schema.go`
  - `internal/db/db.go`
  - `internal/db/db_test.go`
  - `internal/db/testdata/schema_v1.sql`
- Requirement read: `agent-mail/from-orchestrator/to-kernel--board-status.md` and the relevant discovery wording in `mvp-plan.md:37-39`.

## Checks performed

- Verified sequential migration wiring:
  - `schema.CurrentVersion` is now 3.
  - `EnsureCurrent` still migrates one version at a time.
  - `applyMigration` dispatches version 3.
- Verified migration 3 behavior from the SQL in `internal/db/db.go`:
  - Drops and recreates `idx_job_boards_status` around the table rebuild.
  - Rebuilds `job_boards` with `status TEXT NOT NULL DEFAULT 'discovered'`.
  - Allows exactly `discovered`, `active`, and `unsupported`.
  - Copies `board_id`, URL, company, ATS type, status, metadata, and timestamps from the old table.
  - Inserts schema migration version 3 after the rebuild.
- Verified preservation behavior against a v1 fixture plus an unsupported row:
  - Existing active row kept `board_id=1` and `status='active'`.
  - Existing unsupported row kept `board_id=2` and `status='unsupported'`.
- Verified SQLite behavior manually with Python/sqlite3 because the Go toolchain is unavailable in this environment:
  - Omitted board status defaults to `discovered`.
  - Explicit `discovered` status is accepted.
  - Invalid `pending` status is rejected by the check constraint.
  - `idx_job_boards_status` exists after migration.
  - `PRAGMA foreign_key_check` reports no violations.
- Verified test coverage in `internal/db/db_test.go` covers the requested acceptance points:
  - Upgrade from historical schema fixture.
  - Preservation of active and unsupported board IDs/statuses.
  - Default status behavior.
  - Explicit `discovered` acceptance.
  - Invalid status rejection.
- Ran `git diff --check 5bf7ee8^ 5bf7ee8 -- internal/schema internal/db`: passed.

## Verification limitation

- `go test ./internal/db` could not be run here because `go` is not installed (`/usr/bin/bash: line 61: go: command not found`). I substituted a direct sqlite3 migration exercise for the requested table/index/default/check/preservation behavior, but this does not replace running the Go test package in an environment with Go installed.

No blocking findings remain in the reviewed scope.
