# Dispatch: kernel-ui

Implement the shared/blocking Go API and security contract for `ui-overhaul.md`.

## Spec scope

Read `/home/wbw/work/jobs.willwagner.me/ui-overhaul.md`, especially §§1–2, and inspect current handlers/tests before choosing API shapes. Preserve existing Go ownership of authentication, authorization, validation, email delivery, database and business policy. Ensure state-changing API requests retain session-cookie and CSRF protections. Cover existing workflows for signup, email verification, login/logout, dashboard, profile/onboarding, and job list/detail.

## Owned files

- `/home/wbw/work/jobs.willwagner.me/internal/web/server.go`
- `/home/wbw/work/jobs.willwagner.me/internal/web/server_test.go`
- `/home/wbw/work/jobs.willwagner.me/internal/web/coverage_test.go` only if needed for regression/coverage
- `/home/wbw/work/jobs.willwagner.me/docs/ui-api-contract.md` (create; document endpoint, payload, response/error, auth, and CSRF contract for frontend workers)

All other files are read-only. In particular, do not edit `cmd/web/main.go`, Docker/deployment files, frontend files, CI, README, screenshots, fixture tooling, or the spec. The packaging worker owns those. Do not add SPA static serving/fallback in this kernel; the packaging task will implement asset serving after consuming your API contract.

## Worktree and mailbox

Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel-ui`
Task id/branch: `kernel-ui`
Write the final report to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel-ui/to-orchestrator--report-final.md`.
If blocked by an architectural ambiguity, write the question and options to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel-ui/to-orchestrator--query-<topic>.md` and return a query result rather than guessing.

## Acceptance checks

- Add tests proving the API's meaningful success/error paths, auth/session behavior, CSRF enforcement on mutating routes, and no trust of client identity.
- Preserve and pass existing `go test ./internal/web ./cmd/web` (plus any relevant tests you add).
- Run `gofmt` on changed Go files and report exact checks/results.
- Avoid production SMTP, secrets, and real email in tests; use fake notifier.
- Keep changes focused, commit and push branch `kernel-ui`, and open a GitHub PR targeting `master`. Include the PR URL in the report. Do not merge it.
