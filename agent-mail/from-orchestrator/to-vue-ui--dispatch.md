# Dispatch: vue-ui

Implement the Vue SPA UI described by `/home/wbw/work/jobs.willwagner.me/ui-overhaul.md`, using the merged Go API contract at `/home/wbw/work/jobs.willwagner.me/docs/ui-api-contract.md` (master commit `8419179`). Read both before coding.

## Owned files

- `frontend/**` (all Vue source, package manifests/lockfile, Vite config, frontend-only assets/config)
- `docs/ui-component-inventory.md` (create: list interactive components and behaviors so the test task can test all of them)

Do not edit Go files, deployment/container files, CI workflows, README, shared API contract, browser test/fixture/screenshot scripts, or UI spec. Do not edit files outside `frontend/**` and the named inventory doc.

## Requirements

- Create the Vue SPA using shadcn-vue and a maintainable modern visual system. Must implement signup, verification, login, dashboard, profile/onboarding, job matches list, job-match detail.
- Use API from `docs/ui-api-contract.md`; preserve server-side policy authority. Do not trust client user IDs. API mutations need CSRF token from `/api/session` in `X-CSRF-Token` and credentials included.
- Support loading, empty, validation, server failure, and success states as applicable; responsive mobile and desktop; accessible semantics, keyboard interaction, focus states.
- Theme: initial OS preference, manual light/dark control, persist override, follow system until override, minimize initial flash.
- Use a conventional build entry suitable for production packaging; assume `frontend/` and `npm run build` with output under `frontend/dist/`. Tell orchestrator immediately if a conflicting packaging contract needs resolution.
- Don’t make up backend actions not in contract.

## Worktree and mailbox

Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/vue-ui`
Branch/task id: `vue-ui`
Report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-vue-ui/to-orchestrator--report-final.md`
Any blocker query: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-vue-ui/to-orchestrator--query-<topic>.md`.

## Acceptance

- Frontend installs reproducibly from its lockfile and `npm run build` succeeds.
- Provide frontend unit/component test command and tests for UI logic you introduce where practical; separate test infrastructure worker will cover all interactive controls and journeys.
- Document (in component inventory) every interactive component/control, how it behaves, and selectors/accessible labels useful for browser tests.
- Use no production credentials, SMTP or real user data.
- Commit/push branch `vue-ui`, open PR to `master`, do not merge. All PRs require `VERSION` bump. Include exact commands and results in report.
