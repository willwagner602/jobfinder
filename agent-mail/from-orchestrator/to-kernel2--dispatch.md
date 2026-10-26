# Kernel task — shared admin schema v4 and tombstone/signup-disable store

Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel2` (branch `kernel2`, based on current master `e9be093`).
Spec: `/home/wbw/work/jobs.willwagner.me/admin-interface-spec.md`. Plan: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-orchestrator/to-operator--proposed-plan--admin-interface.md`.

## What to build (K only, sequential — leaves wait on this)

Implement the shared storage contracts the admin UI leaves need, exactly at the shape described in "## Task graph — K" in the plan file. Concretely:

1. **Schema v4 migration** (sequential after current v3):
   - Add a single-row `site_config` table (or equivalent minimal shape) holding the global flag `signups_disabled` (boolean, default false). This is what admin's kill-switch writes and what web will read. Keep it domain-typed (INTEGER 0/1 or TEXT 'true'/'false' — pick one, document it) and make the read/write path obvious.
   - Extend the `users` table with the **tombstone** concept distinct from both `suspended` and from a plain dead row. Two acceptable shapes — pick one and document it in the migration comment:
     a) extend `users.status` CHECK to allow `tombstoned` alongside pending/live/suspended, or
     b) add a nullable `tombstoned_at` column that, when set, marks the user as permanently removed (signups with the same email must be rejected; logins must be rejected).
     Whichever you choose must preserve `ON DELETE`/foreign-key behavior on related tables (user_configs, user_history, job_matches).
   - Current `schema.CurrentVersion` should bump `2 → 3 → 4` sequentially, not skip. Fresh databases must end at v4 with the new table/flags present; upgrade from v3 must preserve every existing row exactly.

2. **Store funcs in `internal/admin/admin.go`** (or a closely-related store file within `internal/admin/**`):
   - `IsSignupDisabled(ctx, conn) (bool, error)`
   - `SetSignupDisabled(ctx, conn, want bool) error`
   - `TombstoneUser(ctx, conn, userID)` and `UnTombstoneUser(ctx, conn, userID)` (or a single setter taking the state; pick one shape).
   - Any helper web/web-adjacent code will need to read the flag efficiently (e.g., `ReadSignupDisabled` if the above naming differs).

3. **Tests**: upgrade fixture from v3 → v4 preserving existing rows; fresh-init at v4; both flag helpers; tombstone/untombstone round-trip including the related-row FK preservation check; the CHECK/typed constraint behavior if you went with (b).

## What NOT to touch

- Anything under `internal/matching/**`, `internal/boards/**`, `internal/jobs/**`, `internal/web/**` (besides reading, never editing their files).
- The admin **server**-side HTTP routes/templates (those belong to the U leaf); you're only touching schema/migrations and the internal/admin store layer.
- `deploy/**`, `.github/workflows/**`, `VERSION`, `docs/**`.

## Constraints

- All edits must remain in your owned files (`internal/schema/**`, `internal/db/**` for migrations only, `internal/admin/admin.go` or its close store sibling).
- Run the full local checks you can: `go test ./...`, `bash deploy/check-coverage.sh` on the admin group at minimum, `gofmt -l`, `go vet ./...`. Commit on the `kernel2` branch in your worktree. Do **not** merge.
- When done, write your report to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-kernel2/to-orchestrator--report-final.md` with exact checks run and any tooling limitation.
