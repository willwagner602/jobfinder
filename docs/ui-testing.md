# UI testing and screenshots

This document covers the user-facing Vue UI checks added for the UI overhaul.

## Prerequisites

- Node.js and npm compatible with the checked-in `frontend/package-lock.json`.
- Chromium for Playwright. The scripts use Playwright's bundled browser when available, or a system Chromium from `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`, `CHROMIUM_PATH`, or `/snap/bin/chromium`.
- No production database, SMTP service, credentials, or real verification codes are required.
- Component tests use deterministic in-process mock JSON API fixtures from `frontend/tests/ui-fixtures.ts`.
- Browser E2E tests and screenshot generation use deterministic route-level API fixtures from `scripts/ui-preview/fixtures.json` and `scripts/ui-preview/browser-fixtures.mjs`.
- Fixture data is synthetic and local to test/screenshot tooling; it is not imported by production frontend or Go code.

## Commands

From the repository root:

```sh
cd frontend
npm ci
npm test
npm run test:e2e
npm run build
npm run screenshots
```

`npm test` runs Vitest in jsdom for component/control coverage and focused API/theme checks.

`npm run test:e2e` runs Playwright browser tests against a built Vite preview server. It exercises the actual Vue app in Chromium while intercepting `/api/**` with isolated deterministic responses; it does not contact production services.

`npm run screenshots` builds if needed, serves the built Vue app from `frontend/dist`, drives Chromium with deterministic API fixtures, and writes PNG artifacts under `artifacts/ui-screenshots/` plus `manifest.json`. Generated screenshots are local/CI artifacts, not production assets. Do not commit regenerated artifacts unless a reviewer explicitly asks for reference images.

## Browser-level journeys covered

- Signup submits email/password, navigates to verification, verifies the one-time code, refreshes session, and reaches onboarding.
- Login submits credentials, reaches the dashboard, logs out, and returns to unauthenticated navigation.
- Login code request renders the user-visible delivery status.
- Profile/onboarding editing saves updated fields and shows the server success message.
- Dashboard links navigate to job matches and a job-match detail page.
- Theme switching persists a manual light/dark choice and can return to following the system preference.
- Browser-visible validation and server failures are asserted for signup, verification, and login flows.

## Component/control coverage

The component tests assert user-visible results and validation/failure behavior for:

- Application shell brand/dashboard link, auth-aware navigation, theme toggle, system-theme reset, and logout failure alert.
- Signup email/password controls, client validation, submit action, and server error alert.
- Verification email/code controls, client validation, submit action, and invalid-code alert.
- Login email/password/code controls, login validation, emailed-code action, and credential error alert.
- Dashboard loading, populated, empty, and request-failure states plus dashboard/profile/job links.
- Profile/onboarding text fields, matcher override, required text validation, save action, success message, and save failure.
- Job matches list populated, empty, and request-failure states, including accessible job-card links.
- Job-match detail back link, original listing link, no-summary fallback, and not-found error.

## Screenshot matrix

The screenshot matrix is stored in `scripts/ui-preview/screenshot-matrix.json`. The generator captures every matrix row for both `light` and `dark` themes and for fixed `desktop` (1440×1000) and `mobile` (390×844) viewports.

| Page | Path | States |
| --- | --- | --- |
| Sign up | `/signup` | normal, validation-error, server-error |
| Verify email | `/verify?email=preview.user@example.test` | normal, validation-error, server-error |
| Log in | `/login` | normal (email/password), code-entry (after requesting a code), validation-error, server-error |
| Landing | `/` | normal |
| Dashboard | `/dashboard` | populated, empty, loading, request-failure |
| Profile and onboarding | `/onboarding` | populated, validation-error, request-failure |
| Job matches list | `/jobs` | populated, empty, loading, request-failure |
| Job-match detail | `/jobs/101` | found, no-summary, request-failure |

The fixture data is intentionally synthetic: `preview.user@example.test`, representative profile text, and example job matches. Fixture mode is not wired into production configuration; it is limited to the test files and `scripts/ui-preview` utilities.

## CI integration note

CI integration is owned by the packaging task. The exact commands to wire into CI are:

```sh
cd frontend
npm ci
npm test
npm run test:e2e
npm run build
npm run screenshots
```

If CI uses a system browser instead of Playwright's installed browser, set `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` or `CHROMIUM_PATH` before running the commands.
