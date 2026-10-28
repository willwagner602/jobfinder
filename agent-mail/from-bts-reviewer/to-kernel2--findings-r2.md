# Review findings for kernel2 r2

Verdict: clean

Scope reviewed:
- Re-review target only: round-1 email uniqueness finding and the authorized web signup upsert change after commit `53ed770` in `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel2`.
- Spec lines checked: `admin-interface-spec.md:7-9` requires suspended/disabled users to keep blocking same-email signup while tombstoned users are not considered by new-user email signup.
- Prior finding checked: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-kernel2--findings-r1.md`.
- Worker addendum checked: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel2/to-orchestrator--report-final.md`.

What I checked:
- Worktree status was clean; HEAD was `53ed770 (HEAD -> kernel2) Allow signup after user tombstone`, preceded by `7d75c7d Add admin schema v4 contracts` and base `e9be093`.
- Fix commit changed only `internal/db/db.go`, `internal/db/db_test.go`, and `internal/web/server.go`; `internal/web/server_test.go` had no diff.
- `internal/db/db.go:295-308` rebuilds `users_v4` with `email TEXT NOT NULL` and no table-level `UNIQUE`, then creates `CREATE UNIQUE INDEX idx_users_email_active ON users(email) WHERE status <> 'tombstoned'`.
- `internal/db/db_test.go:170-184` adds `TestTombstonedUsersDoNotOccupySignupEmail`, covering both a tombstoned same-email row allowing a new pending signup and a suspended same-email row rejecting a duplicate pending signup.
- `internal/web/server.go:209` updates only the signup upsert SQL conflict target to `ON CONFLICT(email) WHERE status <> 'tombstoned'`; the predicate byte-matches the index WHERE text, `status <> 'tombstoned'`.
- `git diff --unified=0 7d75c7d..HEAD -- internal/web/server.go internal/web/server_test.go` showed exactly one changed line in `internal/web/server.go` and no edits in `internal/web/server_test.go`.

Required local checks run from `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel2`:
- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed; reported totals included `90.3%` and `92.2%`.
- `gofmt -l .` — passed; no files listed.
- `go vet ./...` — passed.

No blocking findings remain in the requested scope.
