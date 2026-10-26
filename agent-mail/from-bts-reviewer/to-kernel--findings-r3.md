# Kernel review findings — round 3

Verdict: clean

Scope reviewed:
- Review request: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-orchestrator/to-kernel--review-r3.md`.
- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`, especially the shared SQLite startup upgrade requirement and listing versioning requirements at lines 41-43 and 48-71.
- Prior round-2 finding: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-kernel--findings-r2.md`.
- Kernel report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel/to-orchestrator--report-final.md`.
- Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel` at `kernel` / `4e2abe5e13f17ab1ba1b2757a0e945305aef55fd`.
- Changed files reviewed in full: `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/testdata/schema_v1.sql`.
- Supporting file reviewed: `internal/schema/schema.go`.

Checks performed:
- `git status --short`: clean.
- `git log --oneline --decorate -5`: confirmed HEAD is `4e2abe5 Make listing version migration foreign-key safe` on branch `kernel`.
- `git diff --stat HEAD~1..HEAD` and `git diff --name-only HEAD~1..HEAD`: confirmed only `internal/db/db.go`, `internal/db/db_test.go`, and `internal/db/testdata/schema_v1.sql` changed in the round-2 fix.
- `git diff --check HEAD~1..HEAD`: passed.
- Static code review of `EnsureCurrent`, migration 2, the v1 fixture, and tests.
- `go test ./...`: blocked because this environment has no Go toolchain (`go: command not found`).
- Python SQLite reproduction of the v1-to-v2 migration with `PRAGMA foreign_keys=ON` before migration, a dependent `job_matches` row, enforcement disabled only around the rebuild transaction, `PRAGMA foreign_key_check` before commit, enforcement restored after commit, preserved listing version and match rows, and insertion of a later version with the same historical `content_hash`: passed.

Review result:
- The round-2 migration-safety blocker is fixed. `EnsureCurrent` now performs migration work on a pinned SQLite connection, disables foreign-key enforcement before beginning the rebuild transaction, runs migration 2, runs `PRAGMA foreign_key_check` before commit, commits only after a clean check, and restores enforcement afterward.
- The v1 upgrade fixture now includes a user, listing, listing version, and `job_matches` row referencing the listing version. `TestEnsureCurrentUpgradesPreviousSchema` asserts the upgrade succeeds, preserves the version and match data, reports no foreign-key violations, and accepts a repeated historical content hash after upgrade.
- The prior A→B→A fix remains intact. `UpsertListingVersion` compares the incoming hash only to the latest/current version and inserts `current_version + 1` when content differs from the latest version, even if the new hash appeared in an older version. The test still covers A→B→A and expects three preserved version rows.
- No blocking findings remain in the reviewed scope.

Residual risk:
- Full Go verification remains unavailable in this environment because `go`/`gofmt` are not installed. The committed Go tests should still be run in an environment with the Go toolchain before merge/release.
