# Review findings for kernel2 r1

Verdict: findings

Scope reviewed:
- Spec: `/home/wbw/work/jobs.willwagner.me/admin-interface-spec.md`
- Worker report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel2/to-orchestrator--report-final.md`
- Diff: `e9be093..7d75c7d` in `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel2`
- Owned/authorized files changed: `internal/schema/schema.go`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/admin/admin.go`, `internal/admin/admin_test.go`, and the authorized test-only change in `cmd/admin/main_test.go`.

Required checks run locally from the worktree:
- `go test ./...` — passed
- `bash deploy/check-coverage.sh` — passed; reported totals included `90.3%` and `92.2%`
- `gofmt -l .` — passed; no files listed
- `go vet ./...` — passed

Authorization check:
- `cmd/admin/main_test.go` only imports `jobs.willwagner.me/internal/schema` and changes the startup schema assertion from literal `3` to `schema.CurrentVersion` in the failure message and comparison. No other edits were present in that file.

## Important: tombstoned users still block same-email signup

Evidence:
- Spec: `admin-interface-spec.md:9` says tombstoned users "should not be considered during the new user email signup process".
- Implementation: `internal/db/db.go:295-307` rebuilds `users` as `users_v4`, but `internal/db/db.go:297` keeps `email TEXT NOT NULL UNIQUE`.
- Existing signup behavior inserts by email, so this schema contract still treats a tombstoned row as occupying the email globally. A new signup with the same email will hit the unique constraint instead of being allowed to create a new user record.

Why this breaks the spec:
- The spec distinguishes disabled/suspended users from tombstoned users: disabled users must not be able to sign up again with the same email (`admin-interface-spec.md:7`), but tombstoned users must not be considered by the new-user email signup process (`admin-interface-spec.md:9`). Keeping a global unique constraint on `users.email` makes tombstoned rows considered by the database for all future signup inserts.

Smallest correct fix:
- Rebuild `users` without the table-level global `UNIQUE` on `email`, then add a uniqueness constraint/index that excludes tombstoned rows, for example a partial unique index on `email` where `status <> 'tombstoned'` (or equivalent statuses explicitly covering pending/live/suspended). Keep suspended rows unique so disabled users still cannot sign up again. Add a regression test that tombstoning a user permits inserting a new pending user with the same email while a suspended user with that email still blocks signup.

## Notes on clean areas

- v4 migration is sequentially wired after v3 and creates `site_config` with `config_id INTEGER PRIMARY KEY CHECK (config_id = 1)`, `signups_disabled INTEGER NOT NULL DEFAULT 0 CHECK (signups_disabled IN (0, 1))`, and a seeded `(1, 0)` row.
- The `users` rebuild preserves `user_id` and the tests verify related `user_configs`, `user_history`, and `job_matches` rows survive with no foreign-key violations.
- `IsSignupDisabled`, `SetSignupDisabled`, `TombstoneUser`, and `UnTombstoneUser` are implemented and covered by round-trip tests.
- Tombstone and untombstone write distinct history events and unknown-user attempts return `ErrNotFound` without history rows.
