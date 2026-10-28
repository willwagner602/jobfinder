# Dispatch: ui-tests

Implement isolated preview fixtures, browser journey/component tests, and screenshot generation for `/home/wbw/work/jobs.willwagner.me/ui-overhaul.md` §§4–6. Kernel API contract is merged at master commit `8419179`. Read the UI spec, API contract, and Vue worker's `docs/ui-component-inventory.md` before starting. This task is intentionally not dispatched until the component inventory exists.

## Owned files

- `frontend/tests/**` (component and browser test source/config only)
- `scripts/ui-preview/**` (preview fixture/setup utilities)
- `screenshots/**` only for committed reference/readme assets if approved; generated screenshots should be ignored/untracked and output under `artifacts/ui-screenshots/`
- `docs/ui-testing.md` (commands, prerequisites, screenshot page/state matrix)
- `VERSION` (minimal patch bump required by PR policy)

Do not edit app source outside `frontend/tests/**`, frontend package manifests/scripts, Go source/API, deployment/container files, CI workflow, README, shared API/component inventory docs, or spec. If required scripts need adding to `package.json`, query orchestrator rather than editing another task’s files.

## Requirements

- Browser-level journeys: signup and verification, login/logout, profile/onboarding editing, dashboard, jobs list/detail, theme change.
- Component tests for all interactive controls in the component inventory; assert user-visible results and relevant failure/validation behavior.
- Isolated deterministic synthetic fixtures; no production DB/SMTP/credentials, and fixture mode unavailable in production. Use disposable test DB or frontend mock API as appropriate; coordinate isolation with existing app.
- Screenshot command generates all seven pages in light and dark; representative empty/populated/error states as applicable; fixed viewports and deterministic fixtures.
- CI integration is owned by packaging task; provide exact test commands and communicate them to orchestrator.

## Worktree/mailbox

Worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/ui-tests`; branch `ui-tests`; based on Vue PR branch commit `3a506ac`. Report `/home/wbw/work/jobs.willwagner.me/agent-mail/from-ui-tests/to-orchestrator--report-final.md`. Queries use absolute `/home/wbw/work/jobs.willwagner.me/agent-mail/from-ui-tests/to-orchestrator--query-<topic>.md`.

## Acceptance

- All browser/component/screenshot checks are repeatable and require no production secrets or real email.
- Document how to execute tests and screenshot generation and list exact page/theme/state matrix.
- Commit/push branch and open PR to master; do not merge. Ensure required VERSION bump. Report commands/results.