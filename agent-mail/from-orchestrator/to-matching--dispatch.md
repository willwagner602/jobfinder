# Job matching task dispatch

Task/branch/worktree: `matching`; cwd `/home/wbw/work/jobs.willwagner.me/.worktrees/matching`
Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`
Kernel contracts are merged on base commit `70bab3e`. Read `internal/schema/schema.go`, `internal/db/db.go`, `docs/database/verification.md`.
Owned paths only: `internal/matching/**`, `cmd/job-match-search/**`.

Implement spec Implementation Plan §4 and provider-neutral classification acceptance. Separate executable, user high-level filters, user override/global default matcher selection, classifier-agnostic interface, per-user match persistence/raw provider result data, and distinction between match/no-match/failure (failure is never silently treated as a negative). Retry classifier failures with exponential backoff and report exhausted failures. Add deterministic tests for selection, outcomes, retry/backoff, and persistence. Keep stdlib/minimal dependencies; do not edit kernel/go.mod/other scopes; use mailbox for shared-contract questions. Final report `/home/wbw/work/jobs.willwagner.me/agent-mail/from-matching/to-orchestrator--report-final.md`; query files same dir. Commit on branch `matching`; report commit, checks, assumptions/gaps.