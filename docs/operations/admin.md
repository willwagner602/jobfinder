# Admin operations

The admin service is a separate Go process in `cmd/admin`. It uses the shared SQLite schema for users and process history, and an admin-owned `admin_tokens` table for issued operational tokens.

## Runtime

Required environment:

- `JOBS_DB_PATH`: SQLite database path. Defaults to `jobs.sqlite3`.
- `JOBS_ADMIN_TOKEN`: bearer token required for all `/admin` pages.
- `JOBS_ADMIN_ADDR`: listen address. Defaults to `:8081`.

Optional mail outbox environment:

- `JOBS_SMTP_ADDR`
- `JOBS_SMTP_USERNAME`
- `JOBS_SMTP_PASSWORD`
- `JOBS_SMTP_FROM` (defaults to `jobs@willwagner.me`)

Without SMTP settings, token issuance returns the token in the HTTP response and does not attempt live email delivery.

## Routes

- `GET /healthz`: liveness only; no authentication and no dependency readiness checks.
- `GET /admin`: user management page.
- `POST /admin/users/approve`: moves a user to `live` and records user history.
- `POST /admin/users/suspend`: moves a user to `suspended` and records user history.
- `GET /admin/tokens`: token management page.
- `POST /admin/tokens/issue`: issues a token; optionally sends it through the configured outbox.
- `POST /admin/tokens/revoke`: revokes a token.
- `GET /admin/process-history`: shows recent `job_history` rows, status counts, and error JSON.

All admin routes except `/healthz` require `Authorization: Bearer <token>`. The configured `JOBS_ADMIN_TOKEN` is a bootstrap credential for initial setup and emergency access. Tokens issued through `/admin/tokens/issue` are also accepted by protected routes through database validation; revoking an issued token through `/admin/tokens/revoke` immediately removes that token's admin access without revoking the bootstrap credential.
