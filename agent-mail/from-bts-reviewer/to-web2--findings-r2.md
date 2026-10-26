# W2 review findings r2

Verdict: findings

Scope reviewed:
- Spec/context: `admin-interface-spec.md`, prior finding `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-web2--findings-r1.md`, and worker round-1 addendum in `/home/wbw/work/jobs.willwagner.me/agent-mail/from-web2/to-orchestrator--report-final.md`.
- Worktree/head: `/home/wbw/work/jobs.willwagner.me/.worktrees/web`, branch `web`, head `dcb1a6e`, diff base `b5ce2f0`.
- Changed-file scope confirmed exactly:
  - `internal/itests/integration_test.go`
  - `internal/web/server.go`
  - `internal/web/server_test.go`

Checks run from the web worktree:
- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed; the script includes the web group `./cmd/web ./internal/web` and the full script exited 0.
- `gofmt -l .` — no output.
- `go vet ./...` — passed.

Positive confirmations:
- The signup database decision now uses a single pinned SQLite connection from `s.DB.Conn(ctx)` and starts `BEGIN IMMEDIATE` before reading `site_config`, checking tombstone status, upserting `users`, and inserting `web_auth_codes`/`user_history` (`internal/web/server.go:232-273`). Refusal/error paths before commit leave `committed == false` and defer `ROLLBACK` (`internal/web/server.go:241-246`).
- The suspended/pending upsert statement remains the same conflict-target shape, `ON CONFLICT(email) WHERE status <> 'tombstoned' ...`, preserving the prior suspended-email behavior (`internal/web/server.go:267`; covered by `internal/web/server_test.go:174-198`).
- `site_config` is still read as `SELECT signups_disabled FROM site_config WHERE config_id = 1` (`internal/web/server.go:252-257`).
- No new web public/admin route was added; the web route list remains only health/static/signup/verify/login plus authenticated logout/home/onboarding/jobs (`internal/web/server.go:156-172`). The admin signup route drift note did not couple W2 to an admin route.
- The round-1 regression tests are meaningful: `TestSignupTransactionRechecksTombstoneAfterEarlierLiveObservation` observes a live row, tombstones it, then calls the signup helper and asserts refusal/no second row/no code (`internal/web/server_test.go:200-230`); `TestSignupImmediateTransactionBlocksConcurrentTombstone` uses a file-backed DB plus a second `adminConn`, sets low `busy_timeout`, and asserts `admin.Store.TombstoneUser` cannot succeed while the signup helper holds `BEGIN IMMEDIATE` (`internal/web/server_test.go:233-281`). These tests would catch the reverted precheck-before-insert shape.
- Refusal messages remain generic: disabled signups return `Signups are currently disabled`, tombstoned email returns `Email is not available for signup` (`internal/web/server.go:210-218`). Existing unauthenticated admin-boundary integration coverage still asserts `/admin` without credentials is `401 Unauthorized` (`internal/itests/integration_test.go:94-99`).

## Important: signup can still issue an external code before a later transaction rollback

Evidence:
- `createSignupAndIssueCode` opens `BEGIN IMMEDIATE`, defers `ROLLBACK` until `committed` is true, calls `s.issueCodeOnConn`, then executes `COMMIT` (`internal/web/server.go:238-276`).
- `issueCodeOnConn` inserts the auth code and history through the transaction-capable query object, but then immediately calls `s.Notifier.SendCode(ctx, email, code)` before returning to the caller (`internal/web/server.go:294-303`).
- Because `COMMIT` occurs only after `issueCodeOnConn` returns, any `COMMIT` failure or context cancellation between `SendCode` success and `COMMIT` leaves `committed == false`, so the deferred rollback runs after the code has already been sent (`internal/web/server.go:270-276`).

What is wrong:
The database writes are transactional, but code delivery is an external side effect that happens before the transaction commits. A user can receive a signup code whose `web_auth_codes` and `user_history` rows are later rolled back if `COMMIT` fails. The review requirement explicitly asked to flag any path that can “roll back after issuing a code”; this is such a path.

Why the spec/review requires otherwise:
The requested atomicity check covers the whole signup decision, including code/history inserts, and specifically disallows rollback after issuing a code. Sending a code that is not committed leaves the visible signup flow inconsistent: the user receives a verification code that the application cannot verify.

Smallest correct fix:
Keep the site-config read, tombstone check, user upsert, auth-code insert, and history insert inside the pinned `BEGIN IMMEDIATE` transaction, but do not call `Notifier.SendCode` until after `COMMIT` succeeds. Return the generated code from the transactional helper (or split generation/persistence from delivery), commit, mark the transaction committed, then send the notification. If post-commit notification failure must be surfaced, handle it without rolling back the already committed signup/code rows.
