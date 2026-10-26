# Answer to kernel query — web upsert and the partial unique index

Option 1 is authorized: you may make the minimal `internal/web/server.go` SQL-only change needed to make the existing signup upsert target the new partial index correctly.

**Exact authorized scope:**

1. In `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel2/internal/web/server.go`, change *only* the signup upsert statement's conflict clause so the conflict target matches the partial index predicate — e.g. `ON CONFLICT(email) WHERE status <> 'tombstoned' DO UPDATE SET ...` (verify the exact `WHERE` text matches the index predicate you created, byte-for-byte, since SQLite matches the conflict target against the index predicate).
2. **Nothing else in that file.** No behavior change beyond making the existing insert target the intended index; no signing-up-while-tombstoned special handling, no new guards, no endpoint/flow changes, no template edits.
3. Do not touch any other web file.

**Brief rationale for why this is the right call and not scope creep:** this is the same contract-coupling situation as the previous `cmd/admin/main_test.go` authorization — a schema-level contract change has a single, surgical, already-existing consumer whose SQL must reference the new index predicate to remain valid. It does not implement leaf behavior (no UI, no routes, no tombstone-specific logic — it just makes the *existing* insert compile against the *existing* intended semantics). The alternative (leaving full suite red for a later worker) turns one red file into a rolling blocker for every other task, which is worse for the pipeline than this one-line consumer change.

**Also authorized, same scope discipline:** update `internal/web/server_test.go` *only* if any existing test needs its fixture data updated to reflect that a second pending user row with the same email is now possible in the specific tombstoned case (e.g. if a test was asserting the opposite). Do this only if a test is genuinely wrong after the schema fix — do not add new tests in web, and do not edit test helpers beyond what the fix requires. If no test needs changing, don't touch it.

**Explicit non-goals for this authorization (all leaf/U/I work):** enforcing the kill-switch in web, adding per-user pages, adding tombstone UI, adding integration tests. The U/I leaves will do those on their own branches after this kernel lands clean.

After the edit: re-run `go test ./internal/web ./internal/db`, then the full chain (`go test ./...`, `bash deploy/check-coverage.sh`, `gofmt -l`, `go vet ./...`). Commit on `kernel2`, append an addendum to your report noting the second authorization, then finish and report as before.
