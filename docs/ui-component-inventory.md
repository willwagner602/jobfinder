# Vue UI component inventory

This inventory covers interactive controls introduced under `frontend/**` for the user-facing Vue SPA. Browser tests should prefer accessible names and labels listed here.

## Application shell

- Brand/dashboard link: accessible label `jobs.willwagner.me dashboard`; navigates to `/`.
- Primary navigation links:
  - `Dashboard` -> `/` when authenticated.
  - `Profile` -> `/onboarding` when authenticated.
  - `Job matches` -> `/jobs` when authenticated.
  - `Sign up` -> `/signup` when unauthenticated.
  - `Log in` -> `/login` when unauthenticated.
- Theme toggle button: accessible label is dynamic, e.g. `light theme selected`, `dark theme selected`, or `following system (dark)`. Toggles manual light/dark override and persists it in `localStorage` key `jobs-theme`.
- `Use system theme` button: appears only after a manual override; clears `jobs-theme` and resumes OS preference following.
- `Log out` button: authenticated only; sends `POST /api/logout` with `X-CSRF-Token`, refreshes session, and redirects to `/login`.

## Sign up (`/signup`)

- `Email` input (`#signup-email`): required email.
- `Password optional` input (`#signup-password`): optional new password.
- `Create account and send code` button: validates email client-side, sends `POST /api/signup`, shows loading text `Sending code…`, then navigates to `/verify?email=...` on success. Server errors render in an alert.

## Verify email (`/verify`)

- `Email` input (`#verify-email`): required email, prefilled from the `email` query parameter when present.
- `One-time code` input (`#verify-code`): required numeric six-digit code.
- `Verify and continue` button: sends `POST /api/verify`, refreshes session, and redirects to `/onboarding` on success. Server errors render in an alert.

## Log in (`/login`)

- `Email` input (`#login-email`): required before either login action.
- `Password` input (`#login-password`): optional current password.
- `One-time code` input (`#login-code`): optional login code.
- `Log in` button: requires email plus password or code, sends `POST /api/login`, refreshes session, and redirects to `/` on success.
- `Email me a code` button: requires email, sends `POST /api/login` with `sendCode: true`, and renders a success status if delivery was accepted.

## Dashboard (`/`)

- `Update resume and experience` link: navigates to `/onboarding`.
- `View job matches` link: navigates to `/jobs`.
- Recent match cards: links named by visible title/company; navigate to `/jobs/{matchId}`.
- Loading, error, populated, and empty recent-match states are rendered based on `GET /api/dashboard`.

## Profile and onboarding (`/onboarding`)

- `Resume text` textarea (`#resume-text`).
- `Experience bullets` textarea (`#experience-text`).
- `Desired titles, one per line` textarea (`#desired-titles`).
- `Search preferences JSON or notes` textarea (`#search-preferences`).
- `Blocked companies, one per line` textarea (`#blocked-companies`).
- `Matcher override` input (`#matcher-name`).
- `Save and request prior-week match run` button: requires resume or experience text before sending `PUT /api/profile`; shows `Saving…` while pending and renders server success or error messages.

## Job matches list (`/jobs`)

- Job match cards: accessible label `View {title} at {company}`; navigate to `/jobs/{matchId}`.
- `Update profile` link in the empty state: navigates to `/onboarding`.
- Loading, error, populated, and empty states are rendered from `GET /api/jobs`.

## Job-match detail (`/jobs/:matchId`)

- `← Back to job matches` link: navigates to `/jobs`.
- `Original listing` link: appears only when `canonicalUrl` is present; opens in a new tab with `rel="noreferrer"`.
- Loading, error, found, and no-summary states are rendered from `GET /api/jobs/{matchId}`.
