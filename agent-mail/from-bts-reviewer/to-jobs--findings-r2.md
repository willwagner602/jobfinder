# jobs review findings — round 2

Scope reviewed:
- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md` Implementation Plan §3 and acceptance criteria lines 75-78.
- Owned files in `/home/wbw/work/jobs.willwagner.me/.worktrees/jobs`: `internal/jobs/**`, `cmd/job-description-search/**`.
- Diff: `70bab3e..cfb49bec` for owned paths, with specific attention to the round-1 Hacker News production-cache finding.

Checks performed:
- Read the worker report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-jobs/to-orchestrator--report-final.md`.
- Inspected all changed owned files: `internal/jobs/jobs.go`, `internal/jobs/jobs_test.go`, `cmd/job-description-search/main.go`.
- Inspected the shared listing upsert contract used by `internal/jobs` to confirm canonical URL identity and latest-version content-hash semantics.
- Ran `git diff --check 70bab3e -- internal/jobs cmd/job-description-search`; it passed.
- Attempted to run `go test ./internal/jobs ./cmd/job-description-search`, but this review environment has no Go toolchain on `PATH` (`GO_TOOLCHAIN_MISSING`, exit 127), so test execution could not be independently verified here.

Findings:
- No blocking findings remain in the reviewed scope.

Notes:
- The round-1 Hacker News cache finding is addressed in the production path: `Run` now creates one durable `CommentCache` with `NewDurableCommentCache`, reuses it for all Hacker News boards in the run, and persists positive/negative outcomes in `hn_comment_cache` across runs.
- The added tests cover production-path HN cache reuse across multiple HN boards/runs, no negative-cache write after fetch failure, provider parsing for the required sources, Climatebase server-side date filtering, and canonical URL/version behavior.
