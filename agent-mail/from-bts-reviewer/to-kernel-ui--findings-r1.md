# Review findings: kernel-ui r1

Scope reviewed: GitHub PR #1 (willwagner602/jobs.willwagner.me), commit `7208d9b0f3567c7f860615a4d79fce4979a1e91a`, for `ui-overhaul.md` §§1–2 and the kernel-ui dispatch scope.

Files reviewed through the PR branch:
- `docs/ui-api-contract.md`
- `internal/web/server.go`
- `internal/web/server_test.go`

Checks performed:
- Compared API contract and handlers against the spec requirements to keep Go authoritative for auth, authorization, validation, email delivery, database access, and job/business behavior.
- Checked CSRF/session behavior on mutating JSON routes and JSON unauthorized responses for protected API routes.
- Checked route contracts for session bootstrap, signup, verify, login/logout, dashboard, profile update, jobs list, and job detail.
- Checked job detail/list ownership filtering and absence of client-provided user identity trust.
- Checked compatibility with existing Go-template pages and existing route registration.
- Ran `go test ./internal/web ./cmd/web` on the PR branch: passed.
- Ran `git diff --check origin/master...HEAD`: passed.

Verdict: clean. No blocking findings remain in this scope.
