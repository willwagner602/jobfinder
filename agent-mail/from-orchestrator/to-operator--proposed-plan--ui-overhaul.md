# Proposed plan: `ui-overhaul.md`

## Repository and workflow

- Repository: `willwagner602/jobs.willwagner.me`, default/integration branch `master`.
- GitHub CLI is authenticated and `origin` points to the repository.
- Current worktree has pre-existing uncommitted/untracked files, including local `.env` files, artifacts, and the spec. Workers must use dedicated worktrees and must not modify or stage the root worktree's local data.
- All implementation work will be committed on task branches and submitted through GitHub pull requests. No direct merges or root-worktree implementation.

## Task graph

### K — Shared API, SPA hosting, and security contract (kernel; first)
- **Covers:** Spec §§1, 2, 3, and shared portions of §§4–5.
- **Owns:** `internal/web/server.go`, relevant tests under `internal/web/`, `cmd/web/main.go` only if required for asset embedding/config, and the web container build file only for serving the compiled bundle. Kernel must also establish a documented API contract in an owned file under `docs/` or `internal/web/`.
- **Work:** Define safe JSON/API patterns for SPA data/actions that reuse current Go auth and policies; establish SPA/static-asset serving and route fallback without hijacking `/healthz`, APIs, admin, or existing security behavior; define the fixture/test boundary contract used by leaves. Keep current workflows functional during transition as much as possible.
- **Acceptance:** Existing Go tests pass; new handler tests prove auth/CSRF and policy enforcement for API operations; direct SPA route fallback works; API/health/static routes are not captured by the fallback; built asset serving works in the production container shape.
- **Dependencies:** None. Must merge before leaves start.

### V — Vue application, pages, design system, and theme (leaf)
- **Covers:** Spec §§1–3, visual/UI portions of §§7–8.
- **Owns:** Frontend source and package manifests/config (for example `frontend/**`, root frontend config files agreed with kernel), and UI-specific documentation. Does not edit Go handlers, Go tests, fixture infrastructure, CI, deployment container files, or shared docs owned by other tasks.
- **Work:** Implement shadcn-vue-based responsive SPA for signup, verification, login, dashboard, profile/onboarding, job-match list, and job detail; include loading/empty/error/validation behavior and accessible controls; implement system-preferring theme with remembered manual override.
- **Acceptance:** Frontend build and unit/component tests pass; user-visible pages and behaviors connect only through the kernel API; no API security/business rules are reimplemented as authoritative client logic.
- **Dependencies:** K.

### T — Browser tests, component coverage, preview fixtures, and screenshots (leaf)
- **Covers:** Spec §§4–6 and test/screenshot acceptance in §7.
- **Owns:** Browser/component test infrastructure and test sources, screenshot generation scripts/config/output rules, fixture/preview data tooling, and screenshot/preview documentation. Does not edit SPA application source, Go API handlers, container build files, or CI workflow unless explicitly assigned a disjoint file after kernel approval.
- **Work:** Add repeatable synthetic isolated fixtures; browser tests for all listed journeys; component tests for every introduced interactive control in coordination with V's component inventory; screenshot command/matrix for all seven pages, both themes, applicable empty/populated/error states, and fixed viewports.
- **Acceptance:** Tests are meaningful, deterministic, do not send mail or touch production data, and screenshot generation works without production secrets. Preview mode fails closed in production.
- **Dependencies:** K and V (component inventory/source).

### O — CI, production packaging, and integration documentation (leaf)
- **Covers:** Spec §§1, 5–7 build/deployment/documentation requirements.
- **Owns:** `.github/workflows/ci.yml`, `deploy/web.Containerfile`, `README.md` and/or deployment docs (coordinate exact file ownership before dispatch). Does not edit frontend source, Go API handlers, browser test sources, or kernel contract docs.
- **Work:** Build the Vue assets in the production image; serve them without a runtime dev server or third-party asset host; run Go/frontend/test checks in CI; document local dev/build/test/preview/screenshot/production behavior.
- **Acceptance:** Clean production image build includes frontend assets; CI executes required checks; documentation commands match actual scripts.
- **Dependencies:** K and V; coordinate with T for test commands before finalizing CI.

## Parallelization and ownership

K is the only kernel and runs first. After its contracts merge, V begins; T starts after the Vue component inventory is available; O starts after the frontend package/build commands are established. V, T, and O may overlap only after exact file ownership is confirmed disjoint. Every worker gets its own `.worktrees/<task-id>/` worktree and absolute mailbox paths under `/home/wbw/work/jobs.willwagner.me/agent-mail/`.

## Read-only constraints

- All workers treat `ui-overhaul.md`, the existing database schema/policies, and files outside their explicit ownership as read-only.
- No worker may read or copy secrets from `.env` into code, logs, test fixtures, screenshots, commits, or PRs.
- Production data, SMTP, and real email sending are prohibited in tests and screenshot work.
- Keep the admin application untouched except for route compatibility validation.
- Do not stage root-worktree user files/artifacts as part of task commits.

## Risks and assumptions to confirm during kernel work

1. The current Go web app has server-rendered flows and no existing JSON API; SPA API shape and cookie/CSRF handling are shared foundations.
2. Signup verification and login code issuance depend on SMTP. Tests need injected fake notifiers and must never require production mail credentials.
3. The spec asks to test every interactive component; this is broad. T must inventory actual interactive components and provide meaningful tests, excluding decorative elements as stated in the spec.
4. Deterministic local preview data must be isolated from the shared local/prod SQLite volume. The exact implementation mechanism is a kernel contract decision.
5. Production asset packaging and SPA fallback must preserve `/healthz` and the separate admin service routing.

## Approval gate

No worker will be dispatched until the operator approves this plan. After approval, the kernel starts first; leaf tasks follow only after its contracts are reviewed and merged.