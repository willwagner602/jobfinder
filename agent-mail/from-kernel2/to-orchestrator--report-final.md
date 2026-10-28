# Kernel task report — admin schema v4 contracts

## Result

Completed kernel task K for `admin-interface-spec.md` on branch `kernel2`.

Commit: `7d75c7d Add admin schema v4 contracts`

## Built contract surface

### Schema/versioning

- Bumped `schema.CurrentVersion` to `4`.
- Added `schema.UserStatusTombstoned` with value `"tombstoned"`.
- Added schema migration v4 after the existing v3 migration.
- Added `site_config` as a single-row table:
  - `config_id INTEGER PRIMARY KEY CHECK (config_id = 1)`
  - `signups_disabled INTEGER NOT NULL DEFAULT 0 CHECK (signups_disabled IN (0, 1))`
  - `updated_at` timestamp
- Seeded `site_config` row `(1, 0)` during v4 migration so fresh/upgraded DBs default to signups enabled.
- Represented tombstoned users as a distinct `users.status` enum value by rebuilding `users` with `CHECK (status IN ('pending','live','suspended','tombstoned'))`.
- Preserved existing user ids and related FK behavior; related rows remain attached to the same user row because tombstone is a status, not a delete.

### Admin store helpers

Added store-level helpers in `internal/admin/admin.go`:

- `Store.IsSignupDisabled(ctx) (bool, error)`
- `Store.SetSignupDisabled(ctx, want bool) error`
- `Store.TombstoneUser(ctx, userID int64, adminEmail string) error`
- `Store.UnTombstoneUser(ctx, userID int64, adminEmail string) error`

`TombstoneUser` records `admin_tombstoned_user` history and sets status to `schema.UserStatusTombstoned`.
`UnTombstoneUser` records `admin_untombstoned_user` history and restores status to `schema.UserStatusSuspended`, leaving a formerly removed user unable to log in until explicitly approved.

### Tests added/updated

- Fresh v4 init includes `site_config` and defaults `signups_disabled` to `0`.
- `site_config` rejects extra rows and non-0/1 flag values.
- `users.status` accepts `tombstoned` and rejects invalid statuses.
- A true v3 database upgrades to v4 while preserving `users`, `user_configs`, `user_history`, and `job_matches` rows.
- Store tests cover signup flag round-trip.
- Store tests cover tombstone/untombstone round-trip, history events, and preservation of related rows.
- Store tests cover tombstone/untombstone unknown-user `ErrNotFound` behavior without stray history rows.
- Authorized minimal test-only update in `cmd/admin/main_test.go` changes the startup schema assertion from literal `3` to `schema.CurrentVersion`.

## Checks run

Focused checks before the query:

- `gofmt -w internal/schema/schema.go internal/db/db.go internal/db/db_test.go internal/admin/admin.go internal/admin/admin_test.go`
- `go test ./internal/db ./internal/admin` — passed

Full chain after the authorized `cmd/admin/main_test.go` test update:

```sh
gofmt -w cmd/admin/main_test.go internal/schema/schema.go internal/db/db.go internal/db/db_test.go internal/admin/admin.go internal/admin/admin_test.go
go test ./...
bash deploy/check-coverage.sh
test -z "$(gofmt -l cmd/admin/main_test.go internal/schema/schema.go internal/db/db.go internal/db/db_test.go internal/admin/admin.go internal/admin/admin_test.go)"
go vet ./...
```

Result: all passed with exit code 0.

Coverage script summary observed:

- `deploy/check-coverage.sh` completed successfully.
- Final reported group totals included board/job-description coverage at `90.3%` and job-match/matching coverage at `92.2%`.

## Files changed

- `internal/schema/schema.go`
- `internal/db/db.go`
- `internal/db/db_test.go`
- `internal/admin/admin.go`
- `internal/admin/admin_test.go`
- `cmd/admin/main_test.go` — authorized minimal test-only drift-follower edit for `schema.CurrentVersion`

## Notes for leaf tasks

- The signup kill-switch storage contract is `site_config.signups_disabled` as integer `0`/`1` in row `config_id = 1`.
- Tombstone is a user status, not a deleted row. Code that allows login should treat only `live` as allowed; code that checks signup eligibility should ignore `tombstoned` rows.
- Active user email uniqueness is enforced by `idx_users_email_active ON users(email) WHERE status <> 'tombstoned'`, so pending, live, and suspended users still block duplicate signups while tombstoned rows do not.
- Untombstone restores to `suspended`, not `live`, so admin approval is still needed before login can resume.

## Addendum — round 1 review fix

Commit: `53ed770 Allow signup after user tombstone`

Authorization used:

- Review-finding fix scope authorized edits in `internal/db/db.go` and `internal/db/db_test.go`.
- Follow-up authorization in `to-kernel2--answer-web-upsert.md` allowed the minimal SQL-only signup upsert change in `internal/web/server.go` so SQLite can target the new partial unique index.

Changes made:

- Removed the global table-level `UNIQUE` from `users_v4.email` in migration v4.
- Added partial unique index `idx_users_email_active ON users(email) WHERE status <> 'tombstoned'`.
- Updated the existing web signup upsert conflict target to `ON CONFLICT(email) WHERE status <> 'tombstoned' DO UPDATE ...`.
- Verified the predicate text byte-matches between the index and the upsert: `status <> 'tombstoned'`.
- Added `TestTombstonedUsersDoNotOccupySignupEmail` proving a tombstoned same-email row permits a new pending signup while a suspended same-email row still blocks duplicate signup.

Checks run after the fix:

```sh
go test ./internal/web ./internal/db
go test ./...
bash deploy/check-coverage.sh
test -z "$(gofmt -l .)"
go vet ./...
```

Result: all passed with exit code 0. Coverage script completed successfully; final reported group totals again included `90.3%` and `92.2%`.

Files changed in the fix commit:

- `internal/db/db.go`
- `internal/db/db_test.go`
- `internal/web/server.go` — authorized minimal SQL-only change to the signup upsert conflict target
