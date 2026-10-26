# jobs review findings — round 1

Scope reviewed:
- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md` Implementation Plan §3 and acceptance criteria lines 75-78.
- Owned files: `internal/jobs/jobs.go`, `internal/jobs/jobs_test.go`, `cmd/job-description-search/main.go` in worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/jobs`.
- Diff: commit `e6adc5f6e2b03be2f93bd7ccc762947ea38a056f` against its parent for the owned paths.

Checks run:
- `git diff --check HEAD~1..HEAD` passed.
- Attempted `go test ./...`, but this environment has no Go toolchain: `/usr/bin/bash: line 61: go: command not found`.

## Findings

### Important — Hacker News cache is not used by the production run, so per-comment cache does not minimize repeated calls/work

Evidence:
- Spec requires “HN "who's hiring" scraping with per-comment positive/negative cache and retry-on-failure” and “all processes should minimize calls wherever possible” (`/home/wbw/work/jobs.willwagner.me/mvp-plan.md:77-78`).
- `ProviderFor` constructs `HackerNewsProvider{Cache: NewCommentCache()}` every time it is called for an HN board (`/home/wbw/work/jobs.willwagner.me/.worktrees/jobs/internal/jobs/jobs.go:65-66`).
- The production `Run` loop calls `ProviderFor(board.ATSType)` inside the loop for each active board (`/home/wbw/work/jobs.willwagner.me/.worktrees/jobs/internal/jobs/jobs.go:497-503`). There is no longer-lived cache passed into `Run` or persisted between executable runs.
- The cache checked by `HackerNewsProvider.Fetch` is only the provider instance field (`jobs.go:405-430`), so it is discarded after that one board fetch.
- The only test that claims cache behavior reuses the same `HackerNewsProvider` value manually (`/home/wbw/work/jobs.willwagner.me/.worktrees/jobs/internal/jobs/jobs_test.go:88-100`); it does not exercise `Run` or `ProviderFor`, so it would still pass even though the executable path never reuses the cache.

Failure mode:
- On the daily job-description executable path, every run starts with an empty HN positive/negative cache and reparses/reclassifies the same comment ids again. If there are multiple active HN board rows, each row also gets a fresh cache in the same process. That fails the spec’s explicit per-comment cache/minimize-calls requirement in the actual production path.

Smallest correct fix:
- Move HN cache ownership out of `ProviderFor`/single provider instances and into the production runner, e.g. construct one `CommentCache` per `Run` and reuse it for all HN boards, and persist/cache comment outcomes across daily executable runs if the intended cache is process-to-process. Add a test that drives `Run` with HN boards (or otherwise uses `ProviderFor`) and proves a previously seen positive and negative comment id are served from the production cache path rather than only from a hand-reused provider value.
