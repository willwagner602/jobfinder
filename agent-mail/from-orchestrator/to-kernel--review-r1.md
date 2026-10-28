# Kernel review request — round 1

Review task: `kernel`
Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel`
Branch/commit: `kernel` / `6fd5f182e794beb14a609bddafbe86b16810b62a`
Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`
Implementation report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel/to-orchestrator--report-final.md`
Owned paths: `go.mod`, `go.sum`, `internal/db/**`, `internal/schema/**`, `migrations/**`, `cmd/orchestrator/**`, `docs/database/**`.

Adversarially review the task's worktree diff against the spec and dispatch scope. Confirm schema semantics, listing URL identity/content versioning/immutability, migration/startup correctness, DB contracts and tests. Pay special attention to correctness claims that cannot be validated by running Go, since the environment lacks `go`/`gofmt`. Report only specific reproducible findings; verdict must be `clean` or `findings` with severity ranking. Do not modify files. Write the result to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-kernel--findings-r1.md` (or a clearly named report in that directory if clean); return the exact result-file path and verdict.