# Job-description acquisition task dispatch

Task/branch/worktree: `jobs`; cwd `/home/wbw/work/jobs.willwagner.me/.worktrees/jobs`
Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`
Kernel contracts are merged on base commit `70bab3e`. Read `internal/schema/schema.go`, `internal/db/db.go`, `docs/database/verification.md`.
Owned paths only: `internal/jobs/**`, `cmd/job-description-search/**`.

Implement spec Implementation Plan §3 and provider coverage/resilience criteria. Include adapters for Greenhouse, Lever, Ashby (full JD), Workday summaries, Jobvite listings, Climatebase server-side date filtering, and Hacker News who’s hiring scraping with per-comment positive/negative cache and retry-on-failure. Use canonical URL identity and kernel version upsert contract; metadata updates in place, append a version only if hash of title + salary + HTML-stripped/whitespace-collapsed body differs from latest, preserving all old versions and each found_at. Add retries/backoff and deterministic tests/fixtures. Keep stdlib/minimal deps; no edits to kernel, go.mod, or other scopes; request changes via mailbox. Don't claim live scraping verified without credentials/network. Report to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-jobs/to-orchestrator--report-final.md`; query files same directory. Commit on `jobs` and report hash/checks/gaps.