# W2 fix instructions — round 1 finding

Fix the single important finding in `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-web2--findings-r1.md` — the check-then-insert race that can let a tombstoned email slip past the guard via a concurrent admin tombstone.

## Fix requirements

1. Make the tombstone-status check and the signup insert a single atomic write transaction so an admin's tombstone commit cannot interleave between them. Recommended shape (matches the reviewer's suggestion):
   - Begin a SQLite write transaction (`BEGIN IMMEDIATE` so an admin tombstone waits or blocks rather than proceeding).
   - Inside that transaction: `SELECT status FROM users WHERE email = ?` → if tombstoned, roll back and return the existing 403 refusal.
   - Otherwise, still inside the transaction, execute the existing upsert (which already targets the partial index) and issue the verification code.
   - Commit after all three complete. On any error in that chain, roll back before returning.

   You may adjust the store helper structure within web if it's cleaner to add a web-scoped transaction wrapper rather than to inline SQL, but the invariant is: **the status check and the insert cannot be separated by another writer.** `BEGIN IMMEDIATE` or an equivalent write-serialized transaction is exactly that.

2. Keep partial index and suspended/pending upsert semantics unchanged — you're not supposed to change how non-tombstoned emails behave; you're only making the negotiation atomic.

3. **Race regression test.** Now that the transaction makes the race structurally unexploitable, add a test that asserts the sequence the reviewer described: signup A pre-checks email E (not tombstoned), then tombstone E via admin's `Store.TombstoneUser` interleave, then signup A's transaction must recognize tombstoned status at write time and refuse. The exact interleave mechanics — whether you simulate by calling the new code path in two goroutines against the same conn pool, or by a direct injected hook, or by running two real flows — is your design decision, but the test must fail if the fix is reverted. A good shape is a unit test that: (a) inserts a non-tombstoned user row simulating signup-A's early visibility, (b) begins the signup transaction but pauses before commit, (c) in a goroutine (or via direct call) tombstones E from admin side, (d) resumes signup-A's transaction and asserts 403. Note that you cannot easily pause between SQL statements in pure `net/http` handler tests, so the natural split is: split `signupPost` logic into two functions — one that does the transaction-armed decision when passed a callback/interceptor, or expose the transactioned helper and unit-test it directly instead of via HTTP. Choose whichever keeps the production path honest and the test reliable.

4. Then re-run the full chain: `go test ./...`, `bash deploy/check-coverage.sh` (web group must stay >90), `gofmt -l`, `go vet ./...`.

5. Scope stays the same: `internal/web/server.go`, `internal/web/server_test.go`, `internal/itests/integration_test.go` only. Commit on `web`, do not merge. Append an addendum to your report at `/home/wbw/work/jobs.willwagner.me/agent-mail/from-web2/to-orchestrator--report-final.md` noting the fix commit and the new race regression test.
