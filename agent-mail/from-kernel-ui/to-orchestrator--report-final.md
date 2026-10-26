# kernel-ui final report

## Result

Completed the shared Go API/security contract for the UI overhaul kernel scope on branch `kernel-ui`.

- Commit: `7208d9b Add user UI JSON API contract`
- PR: https://github.com/willwagner602/jobs.willwagner.me/pull/1
- Target branch: `master`
- Merge status: not merged

## Built contract surface

Implemented JSON API routes in `internal/web/server.go` while preserving the existing template/form routes:

- `GET /api/session`
- `POST /api/signup`
- `POST /api/verify`
- `POST /api/login`
- `POST /api/logout`
- `GET /api/dashboard`
- `GET /api/profile`
- `PUT /api/profile`
- `GET /api/jobs`
- `GET /api/jobs/{matchId}`

The API keeps Go authoritative for authentication, authorization, validation, email/code delivery, profile persistence, match-run kickoff, job access, and account policy. Authenticated API endpoints derive identity only from the signed `jobs_session` cookie; request bodies do not accept user IDs.

Security contract exposed to frontend workers:

- `GET /api/session` bootstraps auth state and returns `csrfToken`.
- State-changing JSON requests require the existing `jobs_csrf` cookie and matching `X-CSRF-Token` header.
- Protected API endpoints return JSON `401 unauthorized` instead of HTML redirects.
- Mutating API CSRF failures return JSON `403 bad_csrf`.
- Job detail access remains user-scoped; another user's match returns `404 job_not_found`.

Documentation for frontend consumers was added at `docs/ui-api-contract.md`, covering endpoint payloads, responses, errors, auth, and CSRF rules.

## Tests added

Added API coverage in `internal/web/server_test.go` for:

- Signup -> verify -> session -> profile/onboarding update -> jobs list/detail JSON workflow.
- API auth failure behavior.
- API CSRF enforcement on mutating profile route.
- No trust of client identity / user-specific job detail isolation.

## Verification

Commands run from `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel-ui`:

```sh
gofmt -w internal/web/server.go internal/web/server_test.go
go test ./internal/web ./cmd/web
```

Final result:

```text
ok  	jobs.willwagner.me/internal/web	2.063s
ok  	jobs.willwagner.me/cmd/web	0.107s
```

Note: an earlier test run failed due to new test cookie handling for CSRF; the test was corrected and the final required run passed.

## Files changed

- `internal/web/server.go`
- `internal/web/server_test.go`
- `docs/ui-api-contract.md`
