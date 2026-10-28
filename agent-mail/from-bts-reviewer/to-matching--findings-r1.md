# Matching review findings — round 1

Scope reviewed:
- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md` §4, shared schema §5, and acceptance bullet "Provider-neutral LLM classification".
- Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/matching` at `e29963c`.
- Owned paths read in full: `internal/matching/matching.go`, `internal/matching/matching_test.go`, `cmd/job-match-search/main.go`.
- Supporting schema checked: `internal/db/db.go`, `internal/schema/schema.go`.

Checks run:
- `git diff --check HEAD~1..HEAD` produced no output before the next command failed, so no whitespace errors were observed.
- `go test ./...` could not be run in this environment: `/usr/bin/bash: line 61: go: command not found`.
- `command -v go` and `command -v gofmt` produced no paths.

## Findings

### Important — executable default matcher is not an LLM classifier

Evidence:
- Spec requires the job match process to "run the appropriate matcher" with a global default that is initially `jev`, while remaining classifier-agnostic: `mvp-plan.md:45-46`.
- Acceptance explicitly requires "Provider-neutral LLM classification": `mvp-plan.md:79`.
- The only classifier registered by the executable is `keywordClassifier` under matcher name `jev`: `.worktrees/matching/cmd/job-match-search/main.go:47-52`.
- That classifier does not call an LLM/provider at all; it lowercases the title/body and returns `match` when a desired title substring is present, otherwise `no_match`: `.worktrees/matching/cmd/job-match-search/main.go:17-28`.
- The module has no LLM/provider client dependency beyond sqlite: `.worktrees/matching/go.mod:1-6`.

Failure mode:
Running `cmd/job-match-search` with its default `-matcher=jev` records deterministic keyword results as if they were matcher results. Jobs that require actual LLM classification are never sent to an LLM provider, so the executable does not satisfy the provider-neutral LLM classification acceptance criterion.

Smallest correct fix:
Keep the `internal/matching.Classifier` interface, but replace the executable's `jev` registration with a real LLM-backed classifier implementation configured from environment/flags/secrets. Tests should use a fake implementation through the interface, while executable-level coverage should verify that `jev` is wired to the LLM classifier and that provider errors flow through the existing retry/failure path.
