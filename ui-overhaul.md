# UI Overhaul Specification

## Goal

Replace the current user-facing Go-template web UI with a modern, responsive Vue single-page application using **shadcn-vue** components and a coherent design system. The application must support light and dark themes, preserve the existing user workflows, and provide reliable browser tests and reproducible screenshots.

This specification covers the user-facing web application. The separate Go admin interface is out of scope unless explicitly added later.

## Current application context

The web application currently uses Go `html/template` pages, static CSS, and server-side form handlers. Go owns authentication, authorization, persistent data, email verification, and business logic. The UI replacement must preserve these responsibilities and existing security controls; it must not move trusted business rules into the browser.

Before changing routes or data contracts, implementation must inspect the current handlers and tests and document how each existing user workflow maps to the new interface and backend endpoints. Reuse existing server behavior where practical.

## Product requirements

### 1. Frontend and backend architecture

- Build the user interface as a Vue single-page application using shadcn-vue components and an established Vue build toolchain.
- Serve the built frontend assets from the existing Go web service or its deployment unit. Production must not depend on a separate development server or third-party runtime asset host.
- Keep Go as the authority for authentication, authorization, validation, email delivery, database access, and business rules.
- Provide the API endpoints needed by the SPA. Preserve current cookie/session protections and CSRF defenses for state-changing requests; do not expose secrets or trust client-provided user identity.
- Keep the current user-facing workflows and their observable outcomes unless a change is explicitly approved. Provide clear loading, success, empty, validation, and failure states.
- Ensure deep links and browser refreshes on SPA routes resolve correctly without shadowing API routes or health checks.
- Keep the admin application and its routes operational and unaffected by the SPA routing fallback.
- Use accessible semantic controls, keyboard navigation, visible focus states, and labels for form inputs.

### 2. Pages and required states

Implement and visually validate these user-facing pages:

1. Sign up
2. Verify email / enter one-time code
3. Log in
4. Dashboard
5. Profile and onboarding
6. Job matches list
7. Job-match detail

For each page, support the applicable states below:

- Normal, populated state
- Empty state where the page can have no data
- Form validation errors and server-reported errors where applicable
- Loading and request-failure states for asynchronous data operations
- Light and dark themes
- Responsive desktop and mobile layouts

The interface must preserve the current application's available behavior, including signup policy enforcement, email verification, authentication, profile editing, and viewing job matches and details. Do not imply actions that the backend does not support.

### 3. Theme behavior

- On first visit, follow the operating system's light/dark preference.
- Provide an explicit user control to switch between light and dark modes.
- Persist a manual choice across page reloads and future visits on the same browser.
- When no manual choice exists, continue to follow the operating system preference.
- Apply the selected theme before rendering the main UI where practical to avoid a flash of the wrong theme.
- Use consistent design tokens for colors and component states; ensure text and controls remain legible in both modes.

### 4. Local preview data and screenshot mode

- Provide a deterministic local-only preview setup with representative sample users, profiles, and job matches sufficient to render all required pages and states.
- Keep preview data isolated from normal local development data and production data. Prefer a disposable test database or explicitly isolated fixture store.
- Preview mode must be disabled by default and must fail closed in production. Production configuration must not permit enabling preview fixtures or routes.
- Fixture setup and teardown must be repeatable; running it multiple times must not duplicate or corrupt data.
- Include clear run instructions for launching the application in preview mode and generating screenshots.
- Use synthetic data only; do not include real user information, credentials, or real verification codes.

### 5. Functional testing

- Add browser-level tests for the complete user journeys: signup and verification, login and logout, profile/onboarding editing, viewing the dashboard, viewing job matches and details, and changing the theme.
- Add component-level tests for every interactive UI component/control introduced by this project, including form controls, buttons with actions, navigation, dialogs/menus, filters if present, and the theme selector. Purely decorative elements do not require interaction tests.
- Tests must assert meaningful user-visible outcomes, not merely that handlers were called or components rendered.
- Cover success and failure/validation behavior for forms and asynchronous actions where applicable.
- Use isolated deterministic fixtures; tests must not send real email or modify production data.
- Make the test command and prerequisites explicit, and include the relevant checks in CI.

### 6. Screenshot generation

- Provide a repeatable command to generate per-page screenshots using the local preview setup.
- Capture each of the seven required pages in both light and dark themes.
- Capture representative empty, populated, and validation/error states where applicable. Document the page/state matrix so required screenshots are unambiguous.
- Use fixed viewport sizes and deterministic fixtures so screenshots are stable across runs.
- Store generated screenshots in a clearly named, non-production output directory; do not overwrite reference images without an intentional update.
- Ensure screenshot generation can run locally and in CI without production credentials or external email delivery.

## Quality and acceptance criteria

The work is complete when:

- All seven pages are implemented in Vue and preserve the existing user-facing workflows and server-side policy enforcement.
- The built SPA is served through the application deployment and works on direct navigation and refresh, without breaking admin routes or health checks.
- All pages are responsive and usable at mobile and desktop viewport sizes.
- Light/dark theme selection follows the operating system initially, allows a persistent manual override, and is visually readable in both modes.
- Browser tests cover every required journey and component tests cover every introduced interactive control, with meaningful assertions.
- Preview fixtures are deterministic, isolated, synthetic, and impossible to activate in production.
- Screenshot generation produces the documented page/theme/state matrix from fixtures using fixed viewports.
- Existing Go tests, new frontend tests, build checks, and configured CI checks pass.
- Documentation explains local development, preview mode, test execution, screenshot generation, and production build/deployment behavior.

## Out of scope

- Replacing or redesigning the admin interface.
- Changing authentication policy, account status policy, signup policy, or job-matching behavior.
- Adding new user capabilities that the current backend does not provide, unless separately specified.
