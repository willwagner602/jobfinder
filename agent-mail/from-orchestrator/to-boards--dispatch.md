# Job-board discovery task dispatch

Task/branch/worktree: `boards`; cwd `/home/wbw/work/jobs.willwagner.me/.worktrees/boards`
Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`
Kernel contracts are merged on base commit `70bab3e`. Read `internal/schema/schema.go`, `internal/db/db.go`, `docs/database/verification.md`.
Owned paths only: `internal/boards/**`, `cmd/board-search/**`, `data/job-boards.json`.

Implement spec Implementation Plan §2 and Adzuna/SerpAPI limits in acceptance criteria. Separate daily executable, source-isolated processing/workers, configurable providers, candidate parsing/validation, persistence including unsupported ATS boards (never drop candidates merely because unsupported), JSON synchronization, retries/backoff, and tests with deterministic fixtures/fakes. Adzuna max 100 calls/run and 8-day window; SerpAPI max 3 queries × 2 pages. Candidates not auto-activated. Keep stdlib/minimal deps. Do not edit kernel, shared go.mod, or other task scopes; request shared changes/dependency additions through mailbox. Report live external calls as unverified without credentials. Final report `/home/wbw/work/jobs.willwagner.me/agent-mail/from-boards/to-orchestrator--report-final.md`; queries in same folder. Commit on branch `boards`, report hash, verification, and gaps.