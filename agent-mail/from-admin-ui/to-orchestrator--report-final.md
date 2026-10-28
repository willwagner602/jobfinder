# Final report — admin-ui (U task)

## Built

- Added admin user-management routes in `internal/admin/server.go`:
  - `GET /admin/users/{user_id}` per-user data page.
  - `POST /admin/users/tombstone` wired to `Store.TombstoneUser`.
  - `POST /admin/users/untombstone` wired to `Store.UnTombstoneUser`.
  - `POST /admin/signups` wired to `Store.SetSignupDisabled`.
- Updated the admin users page controls:
  - User names link to the per-user data page.
  - Live users show a Disable action; pending/suspended users show Enable; tombstoned users show Restore suspended.
  - Tombstone/delete action is shown for non-tombstoned users.
  - Warning text distinguishes suspension from tombstoning semantics.
- Added the per-user data page showing columns from:
  - `users` including `password_hash`, status, timestamps, and confirmation timestamp.
  - `user_configs` all columns when a row exists.
  - `user_history` all columns for the user.
  - recent `job_matches` all columns, limited to the latest 25 rows.
- Added a shared admin-only HTML/CSS layout embedded in the admin server templates:
  - Collapsible left sidebar with Users, Tokens, and Process history links.
  - Sidebar signup enable/disable control.
  - Sidebar dark-mode toggle.
  - Modern rounded-card styling, restrained colors, light mode default, dark-mode variables.
- Kept changes within owned scope: `internal/admin/server.go` and `internal/admin/server_test.go` only.

## Tests added/extended

Extended `internal/admin/server_test.go` with coverage for:

- User-management page includes detail links, tombstone action, signup toggle, dark-mode/collapsible sidebar affordances, navigation links, and suspend/tombstone warning copy.
- Tombstone and restore routes update user status through the store helpers.
- Signup toggle route updates `site_config.signups_disabled` through the store helper.
- Per-user data page renders user, user config, user history, and recent job match columns/data.

## Checks run

From `/home/wbw/work/jobs.willwagner.me/.worktrees/admin-ui`:

- `gofmt -w internal/admin/server.go internal/admin/server_test.go` — completed.
- `go test ./internal/admin` — passed.
- `go test ./...` — passed.
- `go vet ./...` — passed.
- `go test ./... -cover` — passed with package coverage:
  - `jobs.willwagner.me/cmd/admin`: 90.5%
  - `jobs.willwagner.me/cmd/board-search`: 80.0%
  - `jobs.willwagner.me/cmd/job-description-search`: 90.2%
  - `jobs.willwagner.me/cmd/job-match-search`: 89.7%
  - `jobs.willwagner.me/cmd/orchestrator`: 100.0%
  - `jobs.willwagner.me/cmd/web`: 90.3%
  - `jobs.willwagner.me/internal/admin`: 85.9%
  - `jobs.willwagner.me/internal/boards`: 91.8%
  - `jobs.willwagner.me/internal/db`: 68.3%
  - `jobs.willwagner.me/internal/jobs`: 90.3%
  - `jobs.willwagner.me/internal/matching`: 92.6%
  - `jobs.willwagner.me/internal/schema`: 100.0%
  - `jobs.willwagner.me/internal/web`: 90.8%

## Deviations / notes

- No web-side signup enforcement was added; this task explicitly excluded web-side kill-switch guard work.
- Admin templates/CSS are embedded in `internal/admin/server.go`, matching the existing inline-template pattern. No new framework or user-facing templates were added.
- Recent job matches are limited to 25 rows to keep the detail page bounded while still satisfying the requested recent match view.

## Round 1 findings addendum

Fix commit: `3dddcd0` (`Fix admin UI auth boundaries and coverage`).

Changes made:

- Wrapped the four `GET /admin/users/{approve,suspend,tombstone,untombstone}` method-blocker registrations in `requireAdmin`, so unauthenticated callers receive `401` and authenticated callers using the wrong method receive `405`.
- Added focused tests for every new admin route without a bearer token, including the GET method-blocker paths.
- Added focused tests for authenticated wrong-method `405` behavior, user-detail bad/unknown IDs, missing optional detail rows, tombstone/untombstone bad and unknown IDs, signup toggle bad form/missing config row behavior, signup-status read failure display, and `loadUserDetail` related-row error propagation.

Round-1 checks run from `/home/wbw/work/jobs.willwagner.me/.worktrees/admin-ui`:

- `go test ./cmd/admin ./internal/admin -coverpkg=./cmd/admin,./internal/admin -coverprofile=/tmp/admin.cov && go tool cover -func=/tmp/admin.cov | sort -k3 -n | head -25` — passed; total admin group coverage: **93.5%**.
- `bash deploy/check-coverage.sh` — passed; `coverage-admin.txt` total: **93.5%**.
- `go test ./...` — passed.
- `gofmt -l internal/admin/server.go internal/admin/server_test.go` — passed with no output.
- `go vet ./...` — passed with no output.

Scope note: the round-1 fix changed only `internal/admin/server.go` and `internal/admin/server_test.go`. The coverage script produced untracked `coverage-*.out` and `coverage-*.txt` files in the worktree; they were not committed.
