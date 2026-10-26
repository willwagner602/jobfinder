# Job-description task review request — round 1

Task `jobs`; branch `jobs`; worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/jobs`; commit `e6adc5f6e2b03be2f93bd7ccc762947ea38a056f`.
Spec `/home/wbw/work/jobs.willwagner.me/mvp-plan.md` (Implementation Plan §3; provider and retry criteria in acceptance criteria).
Worker report `/home/wbw/work/jobs.willwagner.me/agent-mail/from-jobs/to-orchestrator--report-final.md`.
Owned paths: `internal/jobs/**`, `cmd/job-description-search/**`.

Review the diff adversarially: provider parsers/endpoints, canonical URL handling, content hash/version flow (latest-version semantics), metadata updates, retry/backoff bounds, HN per-comment cache/retry behavior, job_history outcomes, executable wiring, and test claims. Identify concrete gaps against explicit spec requirements; don't rely on the report. Read-only. Go tooling unavailable. Report findings/clean to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-jobs--findings-r1.md`; return verdict and exact path.