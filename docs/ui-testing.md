# UI testing and screenshots

This document covers the user-facing Vue UI checks added for the UI overhaul.

## Prerequisites

- Node.js and npm compatible with the checked-in `frontend/package-lock.json`.
- No production database, SMTP service, credentials, or real verification codes are required.
- The browser/component tests use deterministic in-process mock JSON API fixtures from `frontend/tests/ui-fixtures.ts`.
- The screenshot generator uses synthetic local-only fixtures from `scripts/ui-preview/fixtures.json` and does not import production code or data.

## Commands

From the repository root:

```sh
cd frontend
npm install
npm test
npm run build
cd ..
node scripts/ui-preview/generate-screenshots.mjs
```

`npm test` runs Vitest in jsdom. The suite covers browser-level journeys through the app shell/router and component-level interactions for all controls listed in `docs/ui-component-inventory.md`.

`node scripts/ui-preview/generate-screenshots.mjs` writes generated artifacts under `artifacts/ui-screenshots/` and a `manifest.json`. Those generated files are local/CI artifacts, not production assets. Do not commit regenerated artifacts unless a reviewer explicitly asks for reference images.

## Browser-level journeys covered

- Signup submits email/password, navigates to verification, verifies the one-time code, refreshes session, and reaches onboarding.
- Login submits credentials, reaches the dashboard, logs out, and returns to unauthenticated navigation.
- Profile/onboarding editing saves updated fields and shows the server success message.
- Dashboard links navigate to job matches and a job-match detail page.
- Theme switching persists a manual light/dark choice and can return to following the system preference.

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
| Log in | `/login` | normal, validation-error, server-error |
| Dashboard | `/` | populated, empty, loading, request-failure |
| Profile and onboarding | `/onboarding` | populated, validation-error, request-failure |
| Job matches list | `/jobs` | populated, empty, loading, request-failure |
| Job-match detail | `/jobs/101` | found, no-summary, request-failure |

The fixture data is intentionally synthetic: `preview.user@example.test`, representative profile text, and example job matches. Fixture mode is not wired into production configuration; it is limited to the test files and `scripts/ui-preview` utilities.

## CI integration note

CI integration is owned by the packaging task. The exact commands to wire into CI are:

```sh
cd frontend && npm ci && npm test && npm run build
node scripts/ui-preview/generate-screenshots.mjs
```

If CI must run a real browser screenshot tool later, add the browser automation dependency and package scripts in the packaging-owned scope; this task intentionally did not edit `frontend/package.json`.
