# Required fix round 1 — PR #3

The final report shows two spec gaps that need to be fixed on the same PR:

1. Journeys are still jsdom-mounted Vue Test Utils tests, not browser-level tests.
2. `generate-screenshots.mjs` emits SVG mockups, not screenshots of the actual Vue pages.

You were explicitly authorized to edit only `frontend/package.json` and lockfile for Playwright/test scripts in `/home/wbw/work/jobs.willwagner.me/agent-mail/from-orchestrator/to-ui-tests--answer-playwright-scope.md`. Please use that authorization. Add real Playwright E2E tests against the built/running Vite SPA with isolated deterministic mocked API responses, and use Playwright to capture actual rendered page screenshots for the required state/theme/viewport matrix. Keep existing jsdom component tests as appropriate. Make the command names explicit and send them to the packaging worker/orchestrator. Do not claim SVG fixtures are screenshots. Run tests/build/screenshots, commit/push same PR. Do not merge.