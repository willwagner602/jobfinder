# Final report — I integration testing framework

Branch/worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/itests` on branch `itests`
Commit: `39687e8 Add subprocess integration tests`

## Built

- Added `internal/itests` as a real-subprocess integration test package.
- The harness builds `cmd/web` and `cmd/admin`, starts them as OS subprocesses, and waits on their real `/healthz` endpoints.
- Each test gets an isolated temp SQLite database, per-test localhost ephemeral ports, and a stdlib fake SMTP server for real signup/verification email-code delivery.
- Tests use stdlib `net/http` with cookie jars and form posts; no browser/UI automation framework is used.
- Added `internal/itests/README.md` documenting how to add future flow tests, naming conventions, harness use, and the CI step convention.
- Added one CI step to `.github/workflows/ci.yml`:
  - `Integration tests` running `go test ./internal/itests/...`

## Contract checks before implementation

Confirmed at current master/head:

- `schema.UserStatusTombstoned` exists in `internal/schema/schema.go`.
- `idx_users_email_active` exists in schema migration v4.
- `site_config.signups_disabled` exists in schema migration v4 and admin store helpers.

## UI flows covered

1. `TestSignupVerifyLoginLogoutHappyPath`
   - Real web subprocess signup, verification via captured SMTP code, authenticated home page, logout, then protected-page redirect to login.
2. `TestLoginJobsListAndDetailShowSeededListing`
   - Real web login, seeded listing/match row, `/jobs` list and `/jobs/{id}` detail content assertions.
3. `TestAdminListUsersRequiresAuthAndBearerTokenListsUsers`
   - Unauthenticated admin user list returns 401; bearer-token authenticated request lists the seeded user.
4. `TestSuspendRejectsLoginAndApproveRestoresLogin`
   - Current disable contract via admin `/admin/users/suspend` rejects web login; `/admin/users/approve` re-enables login.
5. `TestTombstonedUserLoginRejectedAndSignupPolicyDocumented`
   - Tombstoned user cannot log in. Same-email signup rejection is skipped with an explicit message because current read-only web signup allows a new active row for tombstoned emails under the partial unique index contract.
6. `TestAdminProcessHistoryPageRendersCounts`
   - Admin process-history page renders status counts and seeded row details.
7. `TestSignupDisabledSiteConfigBlocksSignupWhenWebEnforcesIt`
   - Sets `site_config.signups_disabled` directly before boot. Skipped with an explicit message because current read-only web signup does not enforce the flag and admin exposes no HTTP config route yet.

## Checks run

From `/home/wbw/work/jobs.willwagner.me/.worktrees/itests`:

- `gofmt -w internal/itests/integration_test.go`
- `go test ./internal/itests/...` — passed
- `go test ./...` — passed
- `go test -v ./internal/itests/...` — passed; showed the two intentional skips above
- `bash deploy/check-coverage.sh` — passed

## Deviations / remaining upstream contracts

- The tombstone signup-rejection portion of the spec cannot be made a passing integration assertion within this task's file scope because current web signup inserts with `ON CONFLICT(email) WHERE status <> 'tombstoned'`, which permits a new active row for a tombstoned email. The test documents and skips that missing behavior instead of faking it.
- The signup-disabled flow cannot be made a passing integration assertion within this task's file scope because current web signup does not read `site_config.signups_disabled`, and current admin routes do not expose signup config controls. The test documents and skips that missing behavior instead of changing read-only web/admin files.

## Files changed

- `.github/workflows/ci.yml`
- `internal/itests/README.md`
- `internal/itests/integration_test.go`
