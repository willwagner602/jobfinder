# Kernel fix response — round 1 finding

Fix exactly one finding from `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-kernel2--findings-r1.md`:

**Important:** tombstoned users must not block same-email signup, but the current v4 rebuild preserves `users.email TEXT NOT NULL UNIQUE`.

Apply the reviewer's smallest-correct-fix within kernel-owned files (`internal/db/db.go` for the migration, `internal/db/db_test.go` for the regression test):

1. In the v4 users rebuild, drop the table-level global `UNIQUE` on `users.email`.
2. Add a partial unique index that excludes tombstoned rows while still keeping suspended/pending/live rows unique — e.g. `CREATE UNIQUE INDEX idx_users_email_active ON users(email) WHERE status <> 'tombstoned'` (or an equivalent predicate covering exactly pending+live+suspended). Critical distinction from the spec: **suspended** users still block re-signup with the same email (spec line 7); only **tombstoned** rows must not.
3. Add the regression test the reviewer asked for: tombstoning a user permits inserting a new `pending` row with the same email, while a suspended user with that email still blocks a new insert (unique-violation on the partial index).
4. Re-run the standard check chain (`go test ./...`, `bash deploy/check-coverage.sh`, `gofmt -l`, `go vet ./...`) and confirm green.
5. Commit on the kernel2 branch, then update your report file with an addendum noting the fix commit hash and the added test.

Stay strictly within the same file set you already own (`internal/schema/**`, `internal/db/**`, `internal/admin/admin.go` + tests). Do not touch anything else. Do not merge.
