# jobfinder

Multi-user job-search MVP (see `mvp-plan.md`): a Go web application plus standalone data-processing services over a shared SQLite database. Users sign up, upload a resume/job history, and receive job matches; admin operators manage users, tokens, and process history.

## Quick facts

| | |
| --- | --- |
| Language | Go 1.23 (stdlib-preferred) |
| Database | SQLite via `github.com/mattn/go-sqlite3` |
| Deployment | docker-compose behind an nginx reverse proxy |
| Live site | jobfinder.you |

## Services

| Service | Entry point | Run cadence | Notes |
| --- | --- | --- | --- |
| web | `cmd/web` | always on | `:8080` — signup/login (emailed one-time code or password), onboarding, job digests |
| admin | `cmd/admin` | always on | `:8081` — `/admin` pages: user approve/suspend, token issue/revoke, process history |
| board-search | `cmd/board-search` | daily | discovers job boards (Adzuna, SerpAPI, HN "who's hiring") into the board table |
| job-description-search | `cmd/job-description-search` | daily | scrapes job listings/descriptions into versioned rows |
| job-match-search | `cmd/job-match-search` | on demand (e.g. from web signup) | classifies job descriptions per user and writes job matches |

All services share only the SQLite schema definitions (`internal/db`, `internal/schema`); each publishes a liveness-only `/healthz`.

## Quickstart

Put runtime configuration in a `.env` next to `docker-compose.yml`, then use the local service launcher:

```sh
./scripts/dev.sh up
```

Open the app at **http://localhost:8090** (admin at **http://localhost:8091/admin**, token `dev-admin-token` by default). Use `./scripts/dev.sh status`, `./scripts/dev.sh logs`, `./scripts/dev.sh down`, and `./scripts/dev.sh reset` to inspect, follow logs, stop it, and wipe the dev database. The launcher uses `docker-compose.dev.yml` and waits for the web health endpoint before reporting ready.

**Dev and production never share state.** The dev stack pins its Compose project name (`jobfinder-dev`) and volume (`jobfinder-dev-data`), so launching from any directory always reuses the same dev database — no more "disappearing accounts" from directory-dependent volume names. It runs on ports 8090/8091 so it can run alongside a production stack on 8080/8081. `docker-compose.yml` (production) remains the source of truth for production service ports; see `deploy/README.md` for the full comparison.

nginx routes public job/signup/login traffic to the web service and `/admin` plus `/healthz` to the admin service. `/healthz` is liveness only and does not check dependency readiness.

## Environment variables

Runtime configuration (compose reads from `.env`):

- `JOBS_ADMIN_TOKEN` — bearer token required for `/admin` pages (bootstrap credential).
- `JOBS_SMTP_ADDR`, `JOBS_SMTP_USERNAME`, `JOBS_SMTP_PASSWORD` — required by the web service at startup; email delivery for issued tokens needs them too.
- `JOBS_SMTP_FROM` — defaults to `jobs@willwagner.me`.
- `JOBS_DB_PATH` (admin) / `JOBS_DB` (web) — SQLite path; `/data/jobs.sqlite3` in compose, backed by the local `./data/jobs.sqlite3` file.
- `JOBS_ADMIN_ADDR` / `WEB_ADDR` — listen addresses; default `:8081` / `:8080`.

Without SMTP settings, token issuance returns the token in the HTTP response instead of emailing it.

## Development

```sh
go test ./...
bash deploy/check-coverage.sh   # >90% coverage gate per service package group
go vet ./...
gofmt -l .                      # should print nothing
```

`deploy/check-coverage.sh` enforces a strict greater-than-90% coverage gate for each shipped service group (admin, web, board-search, job-description-search, job-match-search) and fails on missing packages.

## CI and release

- GitHub Actions runs `go test ./...` and the coverage gate.
- Pull requests must include a `VERSION` file bump or release jobs fail.
- Pushes to `main` record git tags `tested-<sha>` and `tested-<version>` at the tested commit and publish container images to GHCR with commit- and version-derived tags.
- Production credentials and TLS certificates are not checked in; set runtime env vars and mount TLS assets in `deploy/certs/`. The checked-in nginx config is HTTP-only so local/CI validation needs no secrets.

## Schema versioning

- Table shapes live in `internal/schema`, which publishes `CurrentVersion` (currently **6**).
- Migrations run at startup via `cmd/orchestrator` (`orchestrator.Startup` → `db.EnsureCurrent`): the database is upgraded from its stored version to `CurrentVersion` before services run.
- CI verifies a fresh database initializes to `CurrentVersion` and that an upgrade from the checked-in previous-version fixture (`internal/db/testdata/schema_v1.sql`) works.
- Board status values are `discovered`, `unsupported`, and `active` (`internal/schema/schema.go`).
