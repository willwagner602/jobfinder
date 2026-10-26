# Ops/admin review findings — round 1

Scope reviewed:

- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`, especially acceptance criteria for admin user management, health, container deploy, mail integration, GHCR/VERSION/tested-commit release gating, and >90% coverage per service.
- Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/ops` at commit `59c517d`.
- Owned files read: `internal/admin/**`, `cmd/admin/**`, `deploy/**`, `docker-compose.yml`, `.github/workflows/**`, `VERSION`, `docs/operations/**`.

Checks run:

- `git status --short --branch` confirmed `## ops` at `59c517d`.
- `git diff --check && go test ./...` was attempted. `git diff --check` produced no output before the command failed at `go test`; tests could not be run because this environment has no `go` binary (`/usr/bin/bash: line 61: go: command not found`).

## Findings

### Important — Issued/revoked admin tokens are not connected to the admin authorization boundary

Evidence:

- The spec requires admin user management to include “issue/revoke tokens, approve/suspend users” (`mvp-plan.md:80-82`).
- The implementation creates and stores issued admin tokens in `admin_tokens` (`internal/admin/admin.go:69-79`) and exposes issuing/revoking routes (`internal/admin/server.go:159-179`, `internal/admin/server.go:181-195`).
- However every protected admin route authorizes only by comparing the request bearer token with the static `Server.AdminToken` value (`internal/admin/server.go:95-107`), which is loaded from `JOBS_ADMIN_TOKEN` at process startup (`cmd/admin/main.go:15-18`).
- `Store.ValidateToken` validates rows in `admin_tokens` (`internal/admin/admin.go:169-188`), but the route guard never calls it; revocation only updates `admin_tokens` (`internal/admin/admin.go:154-166`) and therefore cannot revoke the static `JOBS_ADMIN_TOKEN` that actually grants access.

Concrete failure mode:

1. Start the admin service with `JOBS_ADMIN_TOKEN=bootstrap`.
2. Authenticate with `Authorization: Bearer bootstrap` and call `POST /admin/tokens/issue`; the service returns a new token.
3. Use that returned token as `Authorization: Bearer <issued-token>` on `GET /admin`; `requireAdmin` compares it to `bootstrap` and returns 401.
4. Revoking the token via `POST /admin/tokens/revoke` also has no effect on the bearer token that grants real admin access.

Why this violates the spec:

The required token issue/revoke admin management does not manage the tokens that control admin access, so the issue/revoke feature cannot grant or remove admin access as required.

Smallest correct fix:

Make the protected-route guard validate bearer tokens through `Store.ValidateToken` and reject revoked/expired rows. If a static bootstrap token is still needed for initial setup, keep it as a separate documented bootstrap path and ensure the admin-issued tokens are accepted and revocation removes their admin access. Add a server test that issues a token, uses it to access `/admin`, revokes it, and verifies subsequent access is rejected.

### Important — The CI coverage gate does not enforce “> 90% coverage per service”

Evidence:

- The spec requires “> 90% coverage per service, with 100% passing tests” (`mvp-plan.md:86`).
- The GitHub Actions workflow first runs tests without coverage (`.github/workflows/ci.yml:25-28`) and then computes/enforces coverage only for `./internal/admin` (`.github/workflows/ci.yml:28-30`).
- The operations documentation repeats the same local coverage commands: `go test ./...`, then `go test ./internal/admin -coverprofile=coverage.out` (`docs/operations/ci-release.md:7-13`).

Concrete failure mode:

A service package outside `internal/admin` can have <90% coverage while `./internal/admin` stays above 90%; the workflow still passes because the only percentage parsed by `awk` comes from `go test ./internal/admin -coverprofile=coverage.out`.

Why this violates the spec:

The release gate is required to enforce coverage per service, not only for one internal package. This means the named acceptance check does not pin the behavior it claims to enforce.

Smallest correct fix:

Run coverage for every service package/binary package that the MVP ships and fail if any service is below 90%. For this task at minimum include the admin executable/package boundary as well as `internal/admin`, and document the exact same per-service command locally.
