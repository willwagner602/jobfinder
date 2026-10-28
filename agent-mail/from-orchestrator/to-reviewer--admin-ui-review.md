# Admin UI review request — U leaf

Review the U leaf work for `admin-interface-spec.md`.

- Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/admin-ui`, branch `admin-ui`, commit `7877166` (diff base: `master` at the time of the worktree creation, i.e. `87e1320`).
- Spec: `/home/wbw/work/jobs.willwagner.me/admin-interface-spec.md`.
- Worker report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-admin-ui/to-orchestrator--report-final.md`.
- Kernel contracts available at HEAD: `Store.IsSignupDisabled / SetSignupDisabled / TombstoneUser / UnTombstoneUser`, `schema.UserStatusTombstoned`, `site_config.signups_disabled`.

**What the reviewer should check:**

1. **Functional coverage against the spec's "User management" bullets, one by one:**
   - Per-user disable/enable buttons (scope: distinct from tombstone; **suspend** is our disable mechanism; **approve** is our enable mechanism — the UI must map to these clearly and not conflate the two).
   - Per-user **tombstone/delete** button (correct semantics: status becomes `tombstoned`, NOT a hard DELETE; still present as a user row so the partial email index can free their email; verify the UI does NOT call an actual DELETE statement path).
   - Per-user data page reachable by clicking the user's name (must show all relevant user data — users + user_configs + user_history + recent job_matches — without requiring admin to know navigation URLs).
   - Global signup enable/disable (wired to `SetSignupDisabled`, state rendered from `IsSignupDisabled`; the control must be visible and labeled clearly in the UI, not hidden in a URL).
2. **Spec styling requirements**: modern look-and-feel, rounded corners, clear but pleasing contrast, default light mode, dark mode available in a *menu* (implement as an explicit toggle in the collapsible left sidebar, not an OS/auto-only thing).
3. **Collapsible left sidebar menu**: present, functional, links adapt to available admin surface (users, tokens, process history at minimum), and it must not obscure content in either mode.
4. **Authorization-boundary correctness**: every new route must still require admin token auth (401 without a valid bearer); no new public/unauthenticated surface.
5. **File scope discipline**: only `internal/admin/**` files should have changed. It must NOT have touched any of: `internal/web/**`, `cmd/web/**`, `internal/schema/**`, `internal/db/**`, `deploy/**`, `.github/**`, `docs/**`, `VERSION`. Confirm with `git diff --name-only 87e1320..7877166`.
6. **Tests**: every new route and the toggle should have coverage; the existing admin test suite must not regress. Run `go test ./...`, `bash deploy/check-coverage.sh` (admin group), `gofmt -l`, and `go vet ./...` locally and report the results.

**Explicit non-goals (not this worker's task, do not fault it for their absence — but DO fault it if it accidentally implemented any of them):** web-side signup kill-switch enforcement (that's a separate authorization I have tracked); job-management page changes (the spec's job history page already exists as `/admin/process-history` and needs no change); integration tests (separate I leaf).

Report findings to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-admin-ui--findings-r1.md`. Verdict clean or severity-ranked, reproducible findings with file:line evidence. Do not edit any files.
