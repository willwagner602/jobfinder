# W2 final report: web signup policy enforcement

Commit: `feb1e83` (`W2 enforce web signup policy`) on branch `web`.

## Files changed

- `internal/web/server.go`
  - Added a pre-insert read of `site_config.signups_disabled` using the kernel v4 single-row schema (`config_id = 1`).
  - When signups are disabled, `POST /signup` now returns `403 Forbidden`, renders `signup.html`, and shows the generic message `Signups are currently disabled` without inserting a user or issuing a verification code.
  - Added a pre-insert tombstoned-email check against `users` for the normalized email and `schema.UserStatusTombstoned`.
  - When a tombstoned email is used, `POST /signup` now returns `403 Forbidden`, renders `signup.html`, and shows the generic message `Email is not available for signup` without inserting a replacement user or issuing a verification code.
  - Left the existing `ON CONFLICT(email) WHERE status <> 'tombstoned'` insert/upsert path intact for non-tombstoned rows.

- `internal/web/server_test.go`
  - Added `TestSignupDisabledBlocksSignup` to prove the site config kill switch blocks signup, creates no user, and sends no code.
  - Added `TestSignupRefusesTombstonedEmail` to prove tombstoned emails are refused, the tombstoned row remains the only row for that email, and no code is sent.
  - Added `TestSignupStillHandlesSuspendedEmailWithExistingUpsert` to prove suspended rows still follow the existing upsert/verification-code behavior and are not blocked by the tombstone-specific precheck.

- `internal/itests/integration_test.go`
  - Removed the skip in `TestSignupDisabledSiteConfigBlocksSignupWhenWebEnforcesIt`; it now asserts `403 Forbidden` and the rendered `Signups are currently disabled` message.
  - Removed the re-signup skip in `TestTombstonedUserLoginRejectedAndSignupPolicyDocumented`; it now asserts `403 Forbidden` and the rendered `Email is not available for signup` message, while the login-rejected assertion remains in place.

## Behaviors demonstrated

- Signup kill switch: demonstrated by `internal/web.TestSignupDisabledBlocksSignup` and `internal/itests.TestSignupDisabledSiteConfigBlocksSignupWhenWebEnforcesIt`.
- Tombstoned-email refusal: demonstrated by `internal/web.TestSignupRefusesTombstonedEmail` and `internal/itests.TestTombstonedUserLoginRejectedAndSignupPolicyDocumented`.
- Existing suspended-email conflict/upsert behavior preserved: demonstrated by `internal/web.TestSignupStillHandlesSuspendedEmailWithExistingUpsert`.

## Checks run

- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed; web coverage group (`./cmd/web ./internal/web`) total: **90.4%**.
- `gofmt -l` via `test -z "$(gofmt -l .)"` — passed with no unformatted files.
- `go vet ./...` — passed.

## Scope/contracts

No contracts were left unhonored. I did not need to touch schema, DB migrations, admin code, deploy/CI files, or any out-of-scope files beyond unskipping/updating the two named integration tests.

## Round 1 finding addendum

Fix commit: `dcb1a6e` (`W2 make signup tombstone check atomic`) on branch `web`.

### What changed

- `internal/web/server.go`
  - Moved the signup disabled check, tombstoned-email status check, signup upsert, auth-code insert, and auth-code history insert into `createSignupAndIssueCode`.
  - `createSignupAndIssueCode` pins a single SQLite connection and executes `BEGIN IMMEDIATE` before reading `site_config` or `users`, so another writer cannot tombstone the email between the tombstone check and signup insert.
  - The helper commits only after code issuance succeeds and rolls back on every error/refusal path.
  - `issueCode` now reuses the same transaction-capable SQL helper for the normal login-code path, preserving existing `user_history.inbound_ip` behavior.
  - Suspended/pending behavior and the existing `ON CONFLICT(email) WHERE status <> 'tombstoned'` upsert remain unchanged.

- `internal/web/server_test.go`
  - Added `TestSignupTransactionRechecksTombstoneAfterEarlierLiveObservation`, which simulates an earlier live observation followed by a tombstone before the transactioned signup decision; the helper must re-read tombstoned status and refuse without creating a second row or sending a code.
  - Added `TestSignupImmediateTransactionBlocksConcurrentTombstone`, which opens a file-backed DB with a second admin connection and verifies `admin.Store.TombstoneUser` cannot commit while signup's `BEGIN IMMEDIATE` transaction is open.

### Checks rerun

- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed; web coverage group (`./cmd/web ./internal/web`) total: **90.7%**.
- `gofmt -l` via `test -z "$(gofmt -l .)"` — passed with no unformatted files.
- `go vet ./...` — passed.

### Scope/contracts

No out-of-scope files were changed in this fix round. The admin route drift note did not require changes here because the W2 integration tests set `site_config.signups_disabled` directly rather than driving the admin signup-toggle route.

## Round 2 finding addendum

Fix commit: `904e975` (`W2 send signup code after commit`) on branch `web`.

### What changed

- `internal/web/server.go`
  - Kept the site-config read, tombstoned-email check, signup upsert, auth-code insert, and auth-code history insert inside the pinned `BEGIN IMMEDIATE` transaction.
  - Split code persistence from delivery: `storeCodeOnConn` now generates and stores the plaintext code and returns it without calling `Notifier.SendCode`.
  - `createSignupAndIssueCode` now commits the transaction, marks it committed, and only then calls `Notifier.SendCode`.
  - If post-commit delivery fails, the helper returns `errCodeDelivery`; `signupPost` surfaces that as the existing `verification code failed` 500 response. The committed signup/code/history rows are deliberately not rolled back after commit and will expire through the normal auth-code expiry behavior.
  - The normal login-code path still sends after persisting by using `issueCodeOnConn`, which wraps `storeCodeOnConn` and then calls the notifier.

- `internal/web/server_test.go`
  - Added `TestSignupSendsCodeAfterCommit`, whose notifier verifies the auth-code row is visible and the database is writable during `SendCode`, proving delivery happens after the signup transaction has committed.
  - Added `TestSignupSendFailureAfterCommitKeepsRows`, which forces `SendCode` to fail and verifies the pending user, auth code, and history rows remain durable while the error returned is `errCodeDelivery`.
  - Added `TestSignupDoesNotSendCodeWhenTransactionFailsBeforeCommit`, which confirms pre-commit refusal paths still do not call the notifier or insert rows.

### Checks rerun

- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed; web coverage group (`./cmd/web ./internal/web`) total: **90.2%**.
- `gofmt -l` via `test -z "$(gofmt -l .)"` — passed with no unformatted files.
- `go vet ./...` — passed.

### Scope/contracts

No out-of-scope files were changed in this fix round. The existing race regression tests, suspended/pending upsert behavior, generic refusal messages, and integration tests remain unchanged.
