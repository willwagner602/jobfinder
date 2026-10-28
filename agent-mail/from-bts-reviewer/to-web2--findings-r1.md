# W2 review findings r1

Verdict: findings

Scope reviewed:
- Spec/context: `admin-interface-spec.md`, remaining-work note, W2 worker final report.
- Diff base/head: `b5ce2f0..feb1e83` in `/home/wbw/work/jobs.willwagner.me/.worktrees/web`.
- Changed files confirmed exactly:
  - `internal/itests/integration_test.go`
  - `internal/web/server.go`
  - `internal/web/server_test.go`
- No deploy/CI, schema/db, admin, or other files are in the commit diff.

Checks run from the web worktree:
- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed.
- `gofmt -l .` — passed with no output.
- `go vet ./...` — passed.

Positive coverage confirmed:
- `signupPost` reads `site_config.signups_disabled` with `config_id = 1` before insert/code issuance and returns 403 rendering `signup.html` with `Signups are currently disabled` when set (`internal/web/server.go:200-208`).
- The existing suspended/pending upsert statement remains present, including `ON CONFLICT(email) WHERE status <> 'tombstoned'` (`internal/web/server.go:230`).
- Unit tests cover disabled signup, tombstoned-email refusal, and preserved suspended-email behavior (`internal/web/server_test.go:113-195`). The disabled and tombstoned unit tests assert response message, no extra row/code issuance; the suspended test asserts redirect and code issuance.
- The two integration tests are unskipped and assert both 403 and rendered message text (`internal/itests/integration_test.go:119-132`, `internal/itests/integration_test.go:147-161`).
- No new public routes were added; the route list is unchanged aside from handler internals (`internal/web/server.go:156-172`).

## Important: tombstoned-email refusal has a check-then-insert race

Evidence:
- `signupPost` checks for a tombstoned row with a standalone `SELECT` before the insert (`internal/web/server.go:210-220`).
- The later insert is still the original upsert, whose conflict target only covers non-tombstoned rows: `ON CONFLICT(email) WHERE status <> 'tombstoned'` (`internal/web/server.go:230`).
- The underlying unique index also excludes tombstoned rows: `CREATE UNIQUE INDEX idx_users_email_active ON users(email) WHERE status <> 'tombstoned'` (`internal/db/db.go:308`).
- Admin tombstoning is a normal status update to `schema.UserStatusTombstoned` (`internal/admin/admin.go:107-108`).

What is wrong:
A signup can pass the tombstoned precheck while the existing user is still non-tombstoned, then an admin tombstone can commit before the signup insert executes. At that point the partial unique index no longer contains the tombstoned row, so the signup insert can create a new pending user with the same email. That violates the requested review condition that tombstoned-email refusal must not be susceptible to a race versus the guard and that a concurrent tombstone-then-signup must not slip past the partial index.

Why the spec requires otherwise:
The remaining-work context says tombstoning permanently blocks re-signup with that email and must be enforced in the web signup handler. The review prompt specifically requires that concurrent tombstone/sign-up interleavings not bypass the guard.

Smallest correct fix:
Make the tombstoned check and signup write one atomic decision that cannot be interleaved with an admin tombstone. For example, acquire a write transaction before checking the email status and keep it through the insert decision, or fold the refusal into a single write-side statement/transaction that observes and protects the final status before creating the pending row. The fix should retain the existing partial index and preserve suspended-row upsert behavior.
