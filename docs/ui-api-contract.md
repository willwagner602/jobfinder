# User UI API contract

This document defines the Go-owned JSON API surface for the user-facing Vue application. Go remains authoritative for authentication, authorization, validation, email delivery, database access, job matching, and account policy. The browser must not send or rely on client-owned user identifiers.

## Security model

- Authentication uses the existing `jobs_session` HTTP-only cookie.
- CSRF uses the existing `jobs_csrf` HTTP-only cookie plus an `X-CSRF-Token` header on every state-changing JSON request.
- Fetch `/api/session` before a mutating request. The response contains `csrfToken`; send that value in `X-CSRF-Token`.
- Mutating endpoints return `403` with `error.code = "bad_csrf"` when the header/cookie pair is missing or invalid.
- Authenticated endpoints derive the current user only from the session cookie. Request bodies do not accept user IDs.
- Unauthenticated API requests to protected resources return `401` JSON instead of redirecting.
- All responses use `Content-Type: application/json; charset=utf-8`.

Error responses use this shape:

```json
{
  "error": {
    "code": "machine_readable_code",
    "message": "Human readable message"
  }
}
```

## Common data shapes

`user`:

```json
{
  "email": "user@example.com"
}
```

`profile`:

```json
{
  "resumeText": "...",
  "experienceText": "...",
  "desiredTitles": "[\"Engineer\"]",
  "searchPreferences": "{...}",
  "blockedCompanies": "[\"ExampleCo\"]",
  "matcherName": "default",
  "lastMatchRunMessage": "Saved; prior-week match run requested."
}
```

`job`:

```json
{
  "matchId": 123,
  "listingId": 456,
  "version": 1,
  "title": "Platform Engineer",
  "company": "ExampleCo",
  "canonicalUrl": "https://example.com/jobs/1",
  "rating": 5,
  "decision": "match",
  "createdAt": "2025-01-01T00:00:00Z",
  "summary": "First 600 characters of the listing body"
}
```

## Endpoints

### `GET /api/session`

Purpose: bootstrap the SPA session state and CSRF token.

Auth: optional.

Response `200`:

```json
{
  "authenticated": true,
  "csrfToken": "token-from-cookie",
  "user": {"email": "user@example.com"}
}
```

If no valid live confirmed session exists, `authenticated` is `false` and `user` is omitted.

### `POST /api/signup`

Purpose: create or update a pending signup and send a signup verification code using the configured server notifier.

Auth: none.

CSRF: required.

Request:

```json
{
  "email": "user@example.com",
  "password": "optional password"
}
```

Response `202`:

```json
{"next": "verify", "email": "user@example.com"}
```

Errors:

- `400 email_required`
- `403 bad_csrf`
- `403 signups_disabled`
- `403 email_not_available`
- `500 verification_code_failed`
- `500 signup_failed`

### `POST /api/verify`

Purpose: verify a signup code, mark the account live/confirmed, and set the session cookie.

Auth: none.

CSRF: required.

Request:

```json
{"email": "user@example.com", "code": "123456"}
```

Response `200`:

```json
{"next": "onboarding", "user": {"email": "user@example.com"}}
```

Errors:

- `400 invalid_code`
- `403 bad_csrf`
- `500 verification_failed`

### `POST /api/login`

Purpose: log in with password, request a login email code, or log in with a login email code.

Auth: none.

CSRF: required.

Request password login:

```json
{"email": "user@example.com", "password": "password"}
```

Request code delivery:

```json
{"email": "user@example.com", "sendCode": true}
```

Request code login:

```json
{"email": "user@example.com", "code": "123456"}
```

Responses:

- Password/code login `200`: `{"next":"dashboard","user":{"email":"user@example.com"}}`
- Code delivery `202`: `{"status":"code_sent"}`

Errors:

- `400 invalid_code`
- `401 invalid_credentials`
- `403 account_not_ready`
- `403 bad_csrf`
- `500 code_failed`

### `POST /api/logout`

Purpose: clear the session cookie.

Auth: required.

CSRF: required.

Response `200`:

```json
{"status": "logged_out"}
```

Errors:

- `401 unauthorized`
- `403 bad_csrf`

### `GET /api/dashboard`

Purpose: provide the dashboard's authenticated user, current profile, and recent job matches.

Auth: required.

Response `200`:

```json
{
  "user": {"email": "user@example.com"},
  "profile": {},
  "recentJobs": []
}
```

Errors:

- `401 unauthorized`
- `500 jobs_failed`

### `GET /api/profile`

Purpose: fetch the current authenticated user's profile/onboarding data.

Auth: required.

Response `200`:

```json
{"user": {"email": "user@example.com"}, "profile": {}}
```

Errors:

- `401 unauthorized`

### `PUT /api/profile`

Purpose: save onboarding/profile data for the current authenticated user and request a prior-week match run.

Auth: required.

CSRF: required.

Request:

```json
{
  "resumeText": "resume text",
  "experienceText": "bullet list",
  "desiredTitles": "Engineer\nStaff Engineer",
  "searchPreferences": "remote only",
  "blockedCompanies": "BadCo",
  "matcherName": "optional matcher"
}
```

Response `200`:

```json
{
  "profile": {},
  "message": "Saved; prior-week match run requested."
}
```

Errors:

- `400 profile_text_required`
- `401 unauthorized`
- `403 bad_csrf`
- `500 profile_save_failed`
- `502 match_kickoff_failed`

### `GET /api/jobs`

Purpose: list up to 100 job matches for the current authenticated user, newest first.

Auth: required.

Response `200`:

```json
{"jobs": []}
```

Errors:

- `401 unauthorized`
- `500 jobs_failed`

### `GET /api/jobs/{matchId}`

Purpose: fetch one job match that belongs to the current authenticated user.

Auth: required.

Path parameter: `matchId` is the server-owned `job_matches.job_match_id`.

Response `200`:

```json
{"job": {}}
```

Errors:

- `401 unauthorized`
- `404 job_not_found` for malformed IDs, unknown matches, or matches owned by another user
- `500 job_failed`
