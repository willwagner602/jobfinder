# operations/admin final report

Task: `operations/admin`
Branch/worktree: `ops` at `/home/wbw/work/jobs.willwagner.me/.worktrees/ops`
Latest commit: `4fc82f6` (`ops: fix admin token auth and coverage gates`)
Prior implementation commit: `59c517d` (`ops: add admin operations service`)

## Built

- Added admin data layer in `internal/admin` for:
  - listing users from the shared `users` table
  - approving users to `live`
  - suspending users to `suspended`
  - recording admin status changes in `user_history`
  - issuing, hashing, validating, listing, and revoking admin-owned tokens
  - reading `job_history` with status counts and error JSON details
- Added admin HTTP service routes:
  - unauthenticated liveness-only `GET /healthz`
  - authenticated `GET /admin` user management page
  - authenticated approve/suspend POST actions
  - authenticated token management page and issue/revoke actions
  - authenticated separate `GET /admin/process-history` page with counts and error details
- Added `cmd/admin` executable that opens/migrates the shared SQLite database, creates the admin token table, configures SMTP from environment, and serves the admin routes.
- Added deployment assets:
  - `docker-compose.yml` for admin service plus nginx reverse proxy
  - `deploy/admin.Containerfile`
  - `deploy/nginx.conf`
  - `deploy/README.md`
- Added CI/release configuration:
  - `.github/workflows/ci.yml`
  - PR gate requiring a `VERSION` file change
  - `go test ./...` plus per-service coverage gate via `deploy/check-coverage.sh`
  - tested-commit tags on main pushes
  - GHCR publishing using commit and VERSION tags
- Added operations documentation:
  - `docs/operations/admin.md`
  - `docs/operations/ci-release.md`
- Added `VERSION` with `0.1.0`.
- Wrote coordination note for the web task at `/home/wbw/work/jobs.willwagner.me/agent-mail/from-ops/to-web--note-mail-outbox.md` documenting the shared `jobs@willwagner.me` SMTP outbox environment boundary.

## Review round 1 fixes

- Fixed issued-token authorization: protected admin routes now accept bearer tokens validated through `Store.ValidateToken` against the `admin_tokens` table.
- Made revocation effective: revoked or expired issued tokens now fail protected-route authorization immediately.
- Kept `JOBS_ADMIN_TOKEN` as a separate bootstrap credential. It is accepted before DB-token validation and is not represented by, or revocable through, the issued-token table.
- Added end-to-end server test `TestIssuedTokenAuthorizesUntilRevokedAndBootstrapRemainsSeparate` covering:
  1. authenticate with bootstrap token,
  2. issue a DB-backed admin token,
  3. access `/admin` with the issued token,
  4. revoke the issued token,
  5. verify the revoked token receives 401,
  6. verify the bootstrap credential still works.
- Added `deploy/check-coverage.sh`, which enforces strict `> 90%` coverage per shipped service group and fails if an expected package group is missing.
- Updated CI to run `go test ./...` and then `bash deploy/check-coverage.sh`.
- Updated operations docs with the same local coverage command and exact service-to-package mapping:
  - admin: `./cmd/admin ./internal/admin`
  - web: `./cmd/web ./internal/web`
  - board-search: `./cmd/board-search ./internal/boardsearch`
  - job-description-search: `./cmd/job-description-search ./internal/jobdescription`
  - job-match-search: `./cmd/job-match-search ./internal/jobmatch`

## Tests/checks

Added tests:

- `internal/admin/admin_test.go`
  - approve/suspend user flow
  - issue/validate/revoke token flow
  - process history ordering/count/error-detail behavior
- `internal/admin/server_test.go`
  - liveness route is unauthenticated and returns JSON
  - admin routes require bearer token
  - process-history page renders status counts and error details
  - issued token authorizes protected routes until revoked; revoked token returns 401; bootstrap token remains separate

Checks run locally:

- `bash -n deploy/check-coverage.sh` passed.
- `git diff --check` passed.
- `git status --short --branch` after commit showed clean `## ops`.
- `git rev-parse --short HEAD` returned `4fc82f6`.
- `go test ./...` was attempted and could not run because this worktree environment has no Go toolchain installed: `/usr/bin/bash: line 61: go: command not found`.
- `gofmt` could not run because `command -v gofmt` returned no path earlier in this worktree.

## Limitations / live verification gaps

- Local Go compilation, formatting, test execution, and coverage execution were not possible in this environment because `go`/`gofmt` are unavailable. CI is configured to run the suite and per-service coverage gate where Go is available.
- The per-service coverage script intentionally fails on missing packages. That means this branch may fail the coverage step until parallel service branches add `./cmd/web`, `./internal/web`, `./cmd/board-search`, `./internal/boardsearch`, `./cmd/job-description-search`, `./internal/jobdescription`, `./cmd/job-match-search`, and `./internal/jobmatch`; this is deliberate per the review request so missing service package groups cannot silently skip the gate at integration time.
- Live SMTP delivery from `jobs@willwagner.me` was not verified because credentials are not available. The implementation uses safe environment-variable configuration and deterministic tests avoid secrets.
- Production TLS/certificate deployment was not verified because production certificates and environment are not available. The nginx/deploy docs call this out and keep secrets outside the repository.
