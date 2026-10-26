# W2 review findings r3

Verdict: clean

Scope reviewed:
- Spec: `admin-interface-spec.md:7-10` for suspended/disabled/tombstoned signup behavior and full-site signup disablement.
- Prior round-2 blocker: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-web2--findings-r2.md`.
- Worker round-2 addendum: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-web2/to-orchestrator--report-final.md`.
- Worktree/head: `/home/wbw/work/jobs.willwagner.me/.worktrees/web`, branch `web`, head `904e975`, diff base `b5ce2f0`.

Diff scope checked:
- `git diff --name-only b5ce2f0..904e975` changes only:
  - `internal/itests/integration_test.go`
  - `internal/web/server.go`
  - `internal/web/server_test.go`
- No route was added; `Server.Routes` still exposes the existing health/static/signup/verify/login/logout/home/onboarding/jobs routes only (`internal/web/server.go:156-172`).

Ordering and rollback review:
- `createSignupAndIssueCode` starts a pinned `BEGIN IMMEDIATE`, reads `site_config`, rechecks the tombstoned status, performs the existing pending-user upsert, and stores auth-code/history rows before commit (`internal/web/server.go:237-279`).
- Delivery no longer happens inside the transaction. `storeCodeOnConn` only generates and stores the code plus history and returns the plaintext code (`internal/web/server.go:303-327`); `createSignupAndIssueCode` executes `COMMIT`, sets `committed = true`, then calls `Notifier.SendCode` (`internal/web/server.go:279-287`).
- The only deferred rollback runs while `committed == false` (`internal/web/server.go:246-250`). The post-commit delivery failure path returns `errCodeDelivery` after `committed = true`, so it cannot trigger rollback (`internal/web/server.go:282-287`).
- No path inside the signup transaction invokes `SendCode`; the transactional helper calls `storeCodeOnConn`, not `issueCodeOnConn` (`internal/web/server.go:275-287`).
- The login-code path remains unchanged in shape: `loginPost` calls `issueCode`, `issueCode` calls `issueCodeOnConn`, and `issueCodeOnConn` persists then sends without being used by the signup transaction (`internal/web/server.go:371`, `internal/web/server.go:514-515`, `internal/web/server.go:292-300`).

Test strength reviewed:
- `TestSignupSendsCodeAfterCommit` uses `commitAwareNotifier`, whose `SendCode` checks that the auth-code row is visible and performs a write against the same database while `SendCode` is running (`internal/web/server_test.go:48-73`, `internal/web/server_test.go:327-353`). This would fail if signup delivery were called before `COMMIT`, because the inserted auth-code row would not be visible outside the transaction and/or the concurrent write would be blocked by the still-open signup transaction.
- `TestSignupSendFailureAfterCommitKeepsRows` forces `SendCode` to fail, asserts `errCodeDelivery`, then verifies the pending user, auth-code row, and `auth_code_issued` history row remain durable (`internal/web/server_test.go:355-380`). This pins the round-2 fix and would catch reverting to rollback-after-delivery behavior.
- `TestSignupDoesNotSendCodeWhenTransactionFailsBeforeCommit` covers a pre-commit refusal path and asserts no notifier call and no user row (`internal/web/server_test.go:383-405`).

Behavior regression checks:
- `site_config.signups_disabled` is still read from `config_id = 1` before signup writes (`internal/web/server.go:257-263`).
- Tombstoned-email status is rechecked inside the `BEGIN IMMEDIATE` transaction (`internal/web/server.go:264-270`), with existing race tests still present (`internal/web/server_test.go:243-325`).
- Suspended/pending upsert behavior keeps the same partial-index conflict target: `ON CONFLICT(email) WHERE status <> 'tombstoned'` (`internal/web/server.go:272`), with suspended-email coverage still present (`internal/web/server_test.go:217-241`).
- Auth boundaries and integration coverage remain in place, including unauthenticated `/admin` rejection (`internal/itests/integration_test.go:88-100`), tombstoned login rejection/signup refusal (`internal/itests/integration_test.go:119-133`), and signup-disabled refusal (`internal/itests/integration_test.go:147-160`).

Checks run from the web worktree:
- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed. The script includes the web group `./cmd/web ./internal/web`; web total reported 90.2%.
- `gofmt -l .` — passed with no output.
- `go vet ./...` — passed with no output.

Notes:
- Running `deploy/check-coverage.sh` produced untracked `coverage-*.out` and `coverage-*.txt` files in the worktree, as the script writes coverage artifacts. No source files were edited.

No blocking findings remain.
