# Proposed build plan — `mvp-plan.md`

## Baseline

- Project root: `/home/wbw/work/jobs.willwagner.me`
- Spec: `/home/wbw/work/jobs.willwagner.me/mvp-plan.md`
- Git was initialized because this directory contained only the spec and the workflow needs isolated worktrees. The spec is committed as baseline `40f7667` (`Add MVP implementation spec`).
- No application code or project instruction files were present at planning time.

## Task graph

### K — Shared database kernel (serial, first)
- **Spec:** Implementation Plan §5; shared assumptions from §§1–4; schema-upgrade acceptance in §5 and health/process-history requirements in acceptance criteria.
- **Owns:** `go.mod`, `go.sum`, `internal/db/**`, `internal/schema/**`, `migrations/**`, `cmd/orchestrator/**`, shared DB contract documentation/tests under `docs/database/**`.
- **Builds:** Initial Go module/layout; SQLite schema for auth, user config/history, boards, canonical job listings and immutable versions, per-user matches, and shared process history; schema version and migration runner; startup upgrade path; common DB types/interfaces and test fixtures.
- **Acceptance:** Fresh DB initializes; a DB at prior schema version upgrades to current; migration/version tests pass; schema supports listing identity by canonical URL and append-only content versions; all processes can record status/items/error details. Tests runnable locally and suitable for CI.
- **Dependencies:** None. All leaves wait for its committed contracts.

### W — Web, authentication, and user workflow (leaf; after K)
- **Spec:** Implementation Plan §1; user-facing job list/detail in §1; auth requirement in acceptance criteria.
- **Owns:** `internal/web/**`, `cmd/web/**`, `templates/**`, `static/**`.
- **Acceptance:** Server-rendered signup/login by password and one-time email code; unconfirmed accounts cannot become live; authenticated pages enforce sessions; onboarding accepts resume and/or experience bullets and persists user profile/config; starts a user-specific prior-week match run through a defined boundary; job list and detail render DB content. Unit/integration tests cover auth boundaries and workflows.
- **Dependency:** K contracts. OTP delivery uses a narrow notifier boundary; transport is coordinated with O.

### B — Job-board discovery (leaf; after K)
- **Spec:** Implementation Plan §2 and discovery upgrade path; Adzuna/SerpAPI constraints in acceptance criteria.
- **Owns:** `internal/boards/**`, `cmd/board-search/**`, `data/job-boards.json`.
- **Acceptance:** Separate daily executable; source-isolated workers; Adzuna capped at 100 calls/run and an 8-day window; SerpAPI limited to 3 queries × 2 pages; candidate boards persist with ATS type and unsupported status instead of being dropped; DB and external JSON stay synchronized; failure/retry behavior tested. Candidate sources are not auto-activated.
- **Dependency:** K contracts.

### D — Job-description acquisition and versioning (leaf; after K)
- **Spec:** Implementation Plan §3; provider coverage and scraping resilience in acceptance criteria.
- **Owns:** `internal/jobs/**`, `cmd/job-description-search/**`.
- **Acceptance:** Separate executable; adapters for Greenhouse, Lever, Ashby, Workday summaries, Jobvite, Climatebase, and HN listings as described; canonical URL is listing identity; metadata refreshes in place; version inserted only when hash of title + salary + normalized body changes; old versions and their `found_at`/full text remain unchanged; retries/backoff and HN per-comment positive/negative cache with retry-on-failure are tested.
- **Dependency:** K contracts.

### M — Matching pipeline (leaf; after K)
- **Spec:** Implementation Plan §4; match schema in §5; provider-neutral classification acceptance criterion.
- **Owns:** `internal/matching/**`, `cmd/job-match-search/**`.
- **Acceptance:** Separate executable; user filters and user override/global default matcher; classifier-neutral interface; match, non-match, and classifier failure are distinct outcomes; failures reported and retried with exponential backoff; per-user matches and raw provider data persist; tests cover retry and failure behavior.
- **Dependency:** K contracts.

### O — Admin, operations, and deployment (leaf; after K)
- **Spec:** admin requirements in §§1 and 5; health/deploy/release/mail/coverage acceptance criteria.
- **Owns:** `internal/admin/**`, `cmd/admin/**`, `deploy/**`, `docker-compose.yml`, `.github/workflows/**`, `VERSION`, operational tests/docs under `docs/operations/**`.
- **Acceptance:** Admin user management can issue/revoke tokens and approve/suspend users; separate process-history admin view exposes status/count/error data; liveness route exists for every service; container compose deployment supports nginx reverse proxy; GHCR release gated on VERSION bump and tags the tested commit; outbox integration supports jobs@willwagner.me; service test coverage targets >90% and tests pass.
- **Dependency:** K contracts. Coordinate email transport boundary with W; admin UI pages live in `internal/admin`, not `internal/web`.

## Parallelization

1. K runs alone. Its task must establish the schema, migration/version contract, shared package interfaces, and passing tests before other work starts.
2. After K is reviewed and merged, W, B, D, M, and O can run concurrently in dedicated worktrees. Owned paths above are disjoint. Cross-task changes to shared contracts must go through an orchestrator query and be handled before editing kernel-owned files.
3. Each task gets an independent review; clean tasks merge to the integration branch. After all merges, run the full test suite and a combined-diff review.

## Shared constraints / read-only guidance

- Treat `mvp-plan.md`, kernel-owned paths, and other workers' owned paths as read-only.
- No worker may edit a different task's scope. No application dependencies or credentials should be invented; document blockers or ask via mailbox.
- Use stdlib where suitable, preserve service separation, and keep provider/classifier logic behind narrow interfaces.
- No provider keys, SMTP credentials, deployment host configuration, or external account access exist in the initial project. Tests should use deterministic fakes; runtime configuration must use environment/configuration inputs and clearly identify what cannot be verified without credentials.
- The spec's >90% per-service coverage, live provider behavior, production email, external deployment, and tested-commit release criteria may need CI/credentials/environment not available in this repository. Workers must report exactly what is and is not verified.

## Risks / decisions to resolve during implementation

- The spec's scope is broad for an MVP and lists many providers without response fixtures, authentication details, or credential sources. Use documented provider contracts and deterministic fixtures; do not silently claim live integration verification.
- Email OTP and outbox requirements meet at the notifier boundary. W owns the OTP call site; O owns the mail transport/outbox implementation. Agree the shared contract before parallel leaf work or route the needed contract change through K.
- The acceptance criterion's phrase “all pages beyond login require user auth” needs public signup/verification routes to remain unauthenticated by design; tests should enforce auth for protected pages while explicitly allowing those onboarding entry points.
- Branch name is currently `master` (Git default); worktrees will be based on the committed baseline. Rename only if needed by project conventions or CI.

## Approval gate

No kernel or leaf workers have been dispatched. Approval of this plan authorizes the kernel dispatch only; leaf workers start only after the kernel contracts and tests are complete and reviewed.