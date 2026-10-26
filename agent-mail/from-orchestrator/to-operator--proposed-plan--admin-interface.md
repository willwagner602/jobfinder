# Proposed build plan — `admin-interface-spec.md`

## Baseline

- Project root: `/home/wbw/work/jobs.willwagner.me`
- Spec: `/home/wbw/work/jobs.willwagner.me/admin-interface-spec.md`
- This is an incremental spec on top of the already-shipped MVP (`mvp-plan.md` complete, all gates green, GHCR publishing at `0.1.1`).
- Existing admin code to be extended: `internal/admin` (store + HTTP server) and `cmd/admin`. Existing tests already cover user list/approve/suspend, tokens, process history, liveness. There is currently **no** tombstone/disable, no per-user detail page, no signup kill-switch, no dark mode/collapsible sidebar styling, and **no integration test framework** in CI (only unit tests + coverage).
- Current CI: `.github/workflows/ci.yml` runs `go test ./...` + `bash deploy/check-coverage.sh` and publishes GHCR images on `VERSION` bump + push. Workflows use `VERSION` bumps on PRs already (per ops task from mvp-plan), so this new work's PRs will inherit those gates.

## Task graph

### K — Shared admin schema extensions (serial, first)
- **Spec:** "User management" bullet list — tombstone state, disable/enable, global signup disable flag; "Job management" — job history columns already exist (`job_history`). Needs: a `site_config` table (or single-row config) to hold `signups_disabled`; a way to distinguish `tombstoned` vs `suspended` (tombstoned users cannot log in AND cannot re-signup with the same email); possibly a `users.status` extension or an adjacent `deleted_at`/`tombstoned_at` column.
- **Owns:** `internal/schema/**`, `internal/db/**` (schema + migrations only), `internal/admin/admin.go` (store funcs touching new fields/columns), plus migration tests.
- **Builds:** Schema v4 migration adding site-config storage; extend user status enum or add tombstone marker (decision below under Risks); store-level `IsSignupDisabled`, `SetSignupDisabled`, tombstone/untombstone helpers; unit tests for each.
- **Acceptance:** Fresh DB initializes to v4; upgrade path from v3 preserves all existing rows; store funcs backed by tests; no schema ambiguity for later work.
- **Dependency:** none. Blocks both leaves.

### U — Admin user-management UI extensions (leaf; after K)
- **Spec:** "User management" section: list users with per-user disable/enable, tombstone/delete, per-user data page on click, global signup enable/disable control, and the styling requirement (modern look-and-feel, rounded corners, light/dark mode, collapsible left sidebar menu).
- **Owns:** `internal/admin/server.go` (user-management routes/pages only), `internal/admin/templates/**` (admin-specific templates only), `internal/admin/static/**` (admin CSS), and tests. **Does not** touch `internal/web/templates/**` or the user-facing site.
- **Builds:** server routes for per-user detail page, tombstone, disable/enable, signup kill-switch; admin templates with the required look; unit tests per route; store wiring from K.
- **Acceptance:** every named control exists and works against the store from K; tests prove unauthenticated 401s, authenticated happy-path, and tombstone-vs-disable semantics; templates build with the same `TemplateDir` mechanism web uses.
- **Dependency:** K.

### I — Integration test framework (leaf; parallel with U)
- **Spec:** "Integration testing framework" section: automated CI-runnable integration tests covering all existing UI flows, and a convention that all future UI flows add tests.
- **Owns:** `internal/itests/**` (new package), `deploy/itests.Containerfile` or a plain script under `deploy/`, additions to `.github/workflows/ci.yml` limited to a single new step invoking this package's entrypoint.
- **Builds:** a real-subprocess integration harness that boots `cmd/admin` (and `cmd/web` if needed for cross-boundary flows like "disabled user can't log in") against a temp SQLite DB, drives real HTTP requests (stdlib `net/http`, no new UI-test framework), and asserts the existing flows: signup→verify→login→logout; login→jobs list/detail (DB content); admin list/users (auth-gated); disable→login rejected; re-enable→login works; tombstone→login rejected **and** signup with same email rejected; process-history page→counts rendered; signup kill-switch→block all POST /signup. Runs on localhost ephemeral ports so it doesn't collide with anything else.
- **Acceptance:** `go test ./internal/itests` passes locally and in the CI step; all currently-specified UI flows mapped to at least one test; gains a documented convention (`itests/README.md`) so "future UI flows must all get tests" is enforceable (reviewers reject new flows without a test).
- **Dependency:** K (needs tombstone/disable state to exist to test).

## Parallelization

1. K runs alone first; commits land on their own branch then rebase/merge into master before leaves start.
2. U and I then run in parallel in disjoint file scopes (templates/static vs a new itests package + a single CI step line).
3. Each task gets an independent review; clean tasks merge; then one combined-diff review across all three.

## File scopes (disjoint)

| Path | K | U | I |
|---|---|---|---|
| `internal/schema/**`, `internal/db/**` migrations | owns | read-only | read-only |
| `internal/admin/admin.go` store funcs (new fields) | owns | read-only | read-only |
| `internal/admin/server.go` (user mgmt pages) | read-only | owns | read-only |
| `internal/admin/templates/**`, `static/**` (new) | read-only | owns | read-only |
| `internal/itests/**` | read-only | read-only | owns |
| `.github/workflows/ci.yml` | read-only | read-only | owns (new step only) |
| `internal/admin/server.go` job-history page read path | read-only | read-only | read-only (already exists; no change needed) |

Existing code (mvp-plan's admin) is treated as **read-only baseline** except for the specific store fields/pages called out above; no worker rewrites existing passing logic.

## Risks / decisions to resolve during implementation

- **Tombstone vs disable:** spec distinguishes the two (disable lets user stay inboxed; tombstone is permanent removal + blocks re-signup with that email). Plan: implement as two distinct store states (a boolean/marker or enum extension), not as one. I'll let K pick the cleanest shape and document it; if spec ambiguity remains, that's a resolver question.
- **Job-history page per spec is already largely built** (`/admin/process-history` shows start/end/status/errors). Reading the spec literally ("job history: start, end, results displayed"), it may need nothing or just a cosmetic retitle. I'll treat it as a read-only existing-page and let the reviewer flag any real delta rather than inventing work.
- **Dark mode toggle scope:** spec says "menu" for the mode switch — I read that as a UI affordance in the collapsible sidebar, not a browser/OS-level thing, so U implements an explicit in-sidebar toggle with `prefers-color-scheme` fallback, not a full custom theming framework.
- **Integration tests vs existing coverage:** our existing 90-percent package-level coverage is not "integration tests" in the spec's sense (it doesn't boot real binaries). I is a distinct, real-subprocess harness, and I'll document that distinction so nobody conflates them.
- **"Signups disabled must also be enforced at the web service":** spec's kill-switch phrase says "all new user signups" — that is the *web* service's signup form, which is web-owned not admin-owned. Blocking at `/admin`'s own UI doesn't satisfy the spec. I is the right place to test this cross-service flow, and U is the right place to add the kicker header/control; the actual web-service enforcement needs either a shared-config read (DB-backed) or a new web-owned flag. I'll treat this as an orchestrator query to resolve at K time rather than guessing, since it changes user-facing web files (which are current web-task-owned, not admin-owned).

## Approval gate

No kernel or leaf workers have been dispatched. Approval of this plan authorizes the kernel dispatch only; leaf workers start only after the kernel contracts and tests are complete and reviewed.
