# Ops/admin review findings — round 2

Verdict: clean

Scope reviewed:

- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`, especially acceptance criteria for admin user management, health, container deploy, mail integration, GHCR/VERSION/tested-commit release gating, and >90% coverage per service.
- Dispatch/review context: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-orchestrator/to-ops--dispatch.md`, round-1 findings, and round-1 fix instructions.
- Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/ops` at commit `4fc82f6`.
- Owned files read: `internal/admin/**`, `cmd/admin/**`, `deploy/**`, `docker-compose.yml`, `.github/workflows/**`, `VERSION`, `docs/operations/**`.

Checks run:

- `git status --short --branch` confirmed a clean `## ops` worktree at `4fc82f6`.
- `git diff --stat 70bab3e..HEAD -- <owned paths>` and `git diff --name-only 70bab3e..HEAD -- <owned paths>` confirmed the reviewed diff is confined to the owned files listed above.
- `git diff --check 70bab3e..HEAD` passed.
- `bash -n deploy/check-coverage.sh` passed.
- `git diff --check 59c517d..HEAD` passed for the review-fix diff.
- `go test ./...` was attempted but could not run in this environment because no Go toolchain is installed: `/usr/bin/bash: line 61: go: command not found`.

Round-1 fix verification:

- Issued-token authorization and revocation are now connected to the admin boundary: protected routes keep `JOBS_ADMIN_TOKEN` as a bootstrap credential, then validate other bearer tokens through `Store.ValidateToken`; revoked or expired DB-backed tokens fail validation. The new server test exercises issue, use, revoke, revoked-token 401, and continued bootstrap access.
- The coverage gate now runs through `deploy/check-coverage.sh`, enumerates the shipped service package groups, uses `go list` so missing packages fail instead of being skipped, runs package-group coverage with `-coverpkg`, and enforces a strict greater-than-90 threshold. CI and local operations docs call the same script and document the service/package mapping.

No blocking findings remain within the requested scope.
