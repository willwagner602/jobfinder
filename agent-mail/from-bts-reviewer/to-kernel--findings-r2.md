# Kernel review findings — round 2

Verdict: findings

Scope reviewed:
- Review request: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-orchestrator/to-kernel--review-r2.md`.
- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`, especially lines 41-43 and 48-71 covering listing versioning and startup schema upgrades.
- Prior finding: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-kernel--findings-r1.md`.
- Updated implementation report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel/to-orchestrator--report-final.md`.
- Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel` at `kernel` / `4dd9efe38dc012595e4db8b6e3c7d2285bd36664`.
- Changed files reviewed in full: `internal/db/db.go`, `internal/db/db_test.go`.
- Supporting files reviewed: `internal/db/testdata/schema_v1.sql`, `internal/schema/schema.go`.
- Diff checks: `git diff --stat HEAD~1..HEAD`, `git diff --name-only HEAD~1..HEAD`, `git diff --check HEAD~1..HEAD`.
- Test attempt: `go test ./...` could not run because this environment has no Go toolchain (`go: command not found`).
- Static migration check: reproduced SQLite behavior with foreign keys enabled; dropping a parent table referenced by existing child rows fails with `FOREIGN KEY constraint failed`.

Prior finding status:
- The A→B→A versioning bug is fixed in the current upsert logic. `internal/db/db.go:270-287` reads only the current version's hash, skips insertion only when that latest hash matches, and otherwise inserts `current_version + 1`.
- The regression test now covers A→B→A. `internal/db/db_test.go:89-139` inserts salary/content A, then B, then A again and expects version 3 with three preserved version rows.
- The fresh v2 schema no longer has `UNIQUE (job_listing_id, content_hash)` on `job_listing_versions` (`internal/db/db.go:193-204`).

## Critical: schema upgrade can fail on real v1 databases with existing matches

- Evidence: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md:71` requires startup to check the database schema version and apply necessary upgrades before services run.
- Evidence: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel/internal/db/db.go:29-33` enables `PRAGMA foreign_keys = ON` before opening the migration transaction.
- Evidence: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel/internal/db/db.go:193-208` implements migration 2 by creating `job_listing_versions_v2`, copying rows, then `DROP TABLE job_listing_versions` and renaming the replacement table.
- Evidence: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel/internal/db/db.go:165-178` defines `job_matches` with a composite foreign key `(job_listing_id, job_listing_version)` referencing `job_listing_versions(job_listing_id, version)`.
- Evidence gap in the upgrade test: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel/internal/db/testdata/schema_v1.sql:54-79` defines both `job_listing_versions` and the `job_matches` foreign key, but `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel/internal/db/testdata/schema_v1.sql:91-92` inserts only a board fixture, so `TestEnsureCurrentUpgradesPreviousSchema` never exercises an existing listing/version/match row.

Failure mode:
1. A v1 database has at least one `job_listing_versions` row and one `job_matches` row referencing that version, which the v1 schema allows and the shared database shape requires.
2. `EnsureCurrent` enables foreign keys and runs migration 2.
3. Migration 2 executes `DROP TABLE job_listing_versions` while `job_matches` still references rows in that table.
4. SQLite rejects the drop with `FOREIGN KEY constraint failed`; the transaction rolls back and startup cannot upgrade the database.
5. This leaves existing databases unable to start on the new schema, contrary to the required startup upgrade path.

Smallest correct fix:
- Rebuild the referenced table in a foreign-key-safe migration. One acceptable pattern is to temporarily disable/defer enforcement outside the migration transaction, rebuild `job_listing_versions`, recreate dependent constraints if needed, run `PRAGMA foreign_key_check`, and only then commit/restore enforcement; another is to rebuild dependent tables in the same migration so no live child foreign key points at the table being dropped.
- Extend `internal/db/testdata/schema_v1.sql` or the upgrade test setup with a v1 `job_listing`, `job_listing_versions`, `users`, and `job_matches` row, then assert `EnsureCurrent` succeeds and preserves both the version row and match row.
