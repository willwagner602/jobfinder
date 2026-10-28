# W2 fix instructions — round 2 finding

Fix the single important finding in `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-web2--findings-r2.md`: a verification code can be sent to a user before its durability is committed, and a later COMMIT failure/cancellation rolls back the auth/history rows beneath an already-delivered code.

## Fix requirements

1. **Move `Notifier.SendCode` to post-commit.** The transactional helper should:
   - Do the site_config read, tombstone check, user upsert, auth-code insert, and history insert inside the same pinned `BEGIN IMMEDIATE` transaction.
   - Return the generated plaintext code to the caller *without sending it*.
   - COMMIT; only after COMMIT succeeds (and `committed` is set before the deferred rollback can fire — verify your commit marker logic covers both success and post-commit-continue paths), call `Notifier.SendCode(ctx, email, code)`.
   - If `SendCode` fails after COMMIT, **do not roll back** the now-durable signup/code rows. Surface the error to the caller in whatever way the surrounding happy-path handler already surfaces delivery errors (or log + error return consistent with existing semantics), so auth codes that exist-but-unreachable are left to their existing expiry behavior rather than resurrected by an impossible rollback. State in a comment why the data side is deliberately not rolled back after commit.

2. **Preserve the pre-commit ordering semantics:** the deferred rollback must still fire for any pre-commit path. Only the send is being moved after commit. Nothing about the check/refusal behavior, partial index upsert, suspended/pending handling, `BEGIN IMMEDIATE` pinning, or generic message text changes.

3. **Test changes.** Adjust the regression tests so they:
   - still assert the original race assertions (tombstone recheck after earlier live observation; concurrent tombstone blocked by the pinned transaction);
   - add one new test proving code delivery happens only after commit: e.g. a fake notifier that captures whether it fired, plus a mock/sqlite hook or injected `sendErr` path that shows rows survive commit even when send errs, plus a test that send is not invoked if commit fails (or is deferred until post-commit); this should fail if the fix is reverted.
   - The existing `TestSignupDisabledBlocksSignup` / `TestSignupRefusesTombstonedEmail` / suspended-upsert tests should not change semantics; if any of them asserted that SendCode must be synchronous with insert inside the tx, update them to the new order.

4. **Run the chain:** `go test ./...`, `bash deploy/check-coverage.sh` (web group must stay >90), `gofmt -l`, `go vet ./...`.

5. **Commit + addendum:** commit on `web`, append an addendum to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-web2/to-orchestrator--report-final.md` with the fix commit hash, the new tests, and a note on what post-commit send failure surfaces to the caller. Scope unchanged: `internal/web/server.go`, `internal/web/server_test.go`, `internal/itests/integration_test.go` only. Do not merge.
