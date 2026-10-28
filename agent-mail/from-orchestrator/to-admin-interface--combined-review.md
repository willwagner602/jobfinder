# Combined admin interface integration review — clean

Scope reviewed: final combined integration state for `admin-interface-spec.md` on current `master`/HEAD, focused only on cross-task seams after the schema, admin UI, integration tests, web signup enforcement, and ops route-alignment streams were merged.

## Verdict

Clean — no blocking integration findings remain.

## What I checked

- Read `/home/wbw/work/jobs.willwagner.me/admin-interface-spec.md` and re-checked the requirements in order:
  - per-user disable/enable and tombstone controls;
  - clicking a user name opens a per-user data page;
  - signup kill-switch blocks public signups from the DB-backed admin-controlled flag;
  - collapsible left sidebar with dynamic admin links;
  - modern light/dark styling;
  - integration tests covering existing UI flows.
- Verified the ops signup-toggle route contract is aligned across implementation and tests:
  - route registration uses `POST /admin/config/signups` in `internal/admin/server.go:92`;
  - handler reads `disabled=true|false` in `internal/admin/server.go:221-236`;
  - sidebar form posts to `/admin/config/signups` and submits the hidden `disabled` field in `internal/admin/server.go:517-520`;
  - admin route test posts both `disabled=true` and `disabled=false` to `/admin/config/signups` in `internal/admin/server_test.go:289-318`.
- Searched for stale `/admin/signups` references. No source or integration-test references remain. The only hits were historical mailbox/report notes under `agent-mail/`, not active code or tests.
- Verified the current integration tests execute and pass, with no skipped tests:
  - `go test -count=1 -v ./internal/itests/...` ran all seven tests and passed, including `TestTombstonedUserLoginRejectedAndSignupPolicyDocumented` and `TestSignupDisabledSiteConfigBlocksSignupWhenWebEnforcesIt`.
- Ran the requested check chain:
  - `go test ./...` passed.
  - `bash deploy/check-coverage.sh` passed; reported totals were above 90% for the checked groups (`90.3%` and `92.2%`).
  - `gofmt -l .` printed `.worktrees/boards/internal/db/db_test.go`, which is inside an untracked nested worktree and not part of current HEAD. I verified the tracked merged Go tree with `git ls-files '*.go' | xargs gofmt -l`, which produced no output.
  - `go vet ./...` passed.

## Requirement sanity-check evidence

- Per-user disable/enable/tombstone controls are routed at `internal/admin/server.go:88-91`, implemented through the status-change handlers at `internal/admin/server.go:159-218`, and rendered in the user table at `internal/admin/server.go:537-539`.
- User names link to the per-user page at `internal/admin/server.go:539`, and that page renders user/config/history/match data at `internal/admin/server.go:543-548`.
- The signup kill-switch is persisted in `site_config` and enforced by the web signup path inside the signup transaction at `internal/web/server.go:257-263`; tombstoned-email refusal is checked at `internal/web/server.go:264-272` before signup writes continue.
- Sidebar, dynamic admin links, collapse control, and dark-mode control are present at `internal/admin/server.go:501-520`, with related styling at `internal/admin/server.go:459-497`.
- Integration coverage includes admin listing/auth, suspend/approve login behavior, tombstoned signup refusal, signup-disabled refusal, job list/detail, process history, and signup/verify/login/logout flows in `internal/itests/integration_test.go:37-162`.

No remaining spec gap found.
