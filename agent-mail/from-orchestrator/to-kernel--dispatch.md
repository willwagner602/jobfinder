# Kernel task dispatch

Task id / branch / worktree: `kernel`
Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`
Owned paths (only): `go.mod`, `go.sum`, `internal/db/**`, `internal/schema/**`, `migrations/**`, `cmd/orchestrator/**`, `docs/database/**`.

## Goal
Implement shared SQLite schema, schema versioning/migrations, shared DB contracts, and startup upgrade check as the kernel foundation for the MVP. The project starts with no source code. Read the spec, especially Implementation Plan §5, and honor cross-service concerns in §§1–4.

## Required scope
- Create an idiomatic Go module/layout, preferring standard library. Choose SQLite driver only if necessary; keep dependencies minimal and explain the choice.
- Provide database shapes needed for users/auth, user config/history, boards (including ATS and unsupported status), canonical job listings and immutable content versions, per-user matches and matcher metadata, and shared job/process history.
- Listing identity is canonical URL, never title. Version history must support immutable prior rows; content hash is based on title + salary + normalized body text. Store full version text, each version's found time, nullable source `posted_at`, and metadata that can refresh independently.
- Implement schema version tracking and an orchestrator startup path that upgrades an older schema before services run. Include a prior-version fixture/test proving upgrade to current version. Include fresh database initialization tests.
- Define reusable narrow types/interfaces/helpers needed by independently-running services; avoid coupling service logic into this package.
- Record process history status, started/finished timestamps, item count, and structured/error blob fields.

## Acceptance checks
- `go test ./...` passes.
- Tests cover fresh initialization, previous schema upgrade, schema version consistency, and key uniqueness/immutability invariants.
- Document exact local verification commands under `docs/database/`.
- Keep implementation self-contained in owned paths. Do not edit the spec, plan/mailbox files, or any unowned path.

## Mailbox
Write a short final report to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel/to-orchestrator--report-final.md` before exiting. If blocked by an ambiguity or scope issue, write `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel/to-orchestrator--query-<topic>.md` with context, options, and recommendation, then exit with query status. Do not message directly except by pointing to mailbox files.

## Git and handoff
Work in `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel`. Commit the completed task on branch `kernel`. Report commit hash, files changed, tests run/results, assumptions, and any unresolved issues. Keep all work limited to the owned paths. The orchestrator will review this task before dispatching dependent leaf workers.