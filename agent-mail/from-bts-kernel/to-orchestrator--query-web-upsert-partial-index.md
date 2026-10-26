# Query: partial unique index fix requires out-of-scope web upsert change for full checks

I applied the requested scoped schema fix in `internal/db/db.go` and regression test in `internal/db/db_test.go`:

- v4 `users_v4.email` no longer has table-level `UNIQUE`.
- v4 creates `CREATE UNIQUE INDEX idx_users_email_active ON users(email) WHERE status <> 'tombstoned'`.
- Added `TestTombstonedUsersDoNotOccupySignupEmail`, which verifies:
  - a tombstoned row does not block a new pending row with the same email;
  - a suspended row still blocks a new pending row with the same email.

Focused scoped check:

```sh
go test ./internal/db
```

Result: passed.

However, the required full check chain now fails in existing `internal/web` signup tests:

```text
--- FAIL: TestSignupSucceedsWithoutPassword (0.00s)
    coverage_test.go:840: passwordless signup status = 500 location = ""
--- FAIL: TestSignupVerifyLoginAndOnboarding (0.10s)
    server_test.go:66: signup did not send verification code
FAIL	jobs.willwagner.me/internal/web	1.276s
```

Likely cause: `internal/web/server.go` currently inserts with:

```sql
INSERT INTO users(email, password_hash, status) VALUES (?, ?, ?)
ON CONFLICT(email) DO UPDATE SET ...
```

SQLite cannot infer a partial unique index from `ON CONFLICT(email)` unless the conflict target includes the matching `WHERE` predicate. With only `idx_users_email_active ON users(email) WHERE status <> 'tombstoned'`, the web signup insert fails with a conflict-target mismatch before it can send a verification code.

## Conflict with scope

The fix instructions say to stay within `internal/schema/**`, `internal/db/**`, `internal/admin/admin.go` + tests, and specifically requested edits only in `internal/db/db.go` and `internal/db/db_test.go`. `internal/web/**` is explicitly out of scope/read-only from the original dispatch.

## Options

1. Authorize the minimal out-of-scope `internal/web/server.go` SQL change to make signup target the partial index, e.g. `ON CONFLICT(email) WHERE status <> 'tombstoned' DO UPDATE ...`. This should restore full-suite green and align signup behavior with the new schema contract.
2. Keep this worker strictly scoped to `internal/db/**`; I can commit the schema/test fix with `go test ./internal/db` green, but the required `go test ./...` will remain red until a web-scoped worker updates the signup upsert.

## Recommendation

Authorize the minimal `internal/web/server.go` SQL update in this branch, because the new schema contract and existing signup upsert are directly coupled. Without it, the requested partial unique index cannot coexist with the required full check chain.
