# Container deployment

The web and admin services share the SQLite database volume. Public job, signup, and login routes are served by web; `/admin` and `/healthz` are served by admin.

## Frontend build behavior

The production web image builds the Vue frontend inside a Node builder stage with:

```sh
cd frontend
npm ci
npm run build
```

The resulting `frontend/dist/` files are copied into the final web image at `/app/static`, and the runtime image sets `SPA_ASSET_DIR=/app/static` so the Go web service serves the production asset bundle from that exact path. The runtime image contains the Go web binary, templates, built static assets, and CA certificates only; it does not include Node, npm, Vite, or the development server.

For local frontend checks outside Docker, run the same commands from `frontend/`. The frontend package also provides `npm test` for component/unit checks. Browser-test and screenshot commands should be run as documented by the frontend test tooling once those scripts are present; production deployment does not require preview fixtures or production credentials for frontend asset builds.

## Required runtime configuration

- `JOBS_ADMIN_TOKEN`: bearer token required for `/admin` pages.
- `JOBS_SMTP_ADDR`, `JOBS_SMTP_USERNAME`, `JOBS_SMTP_PASSWORD`: required by the web service at startup for the jobs@jobfinder.you outbox. Set all three before starting the deployment. The admin service can start without SMTP configured, but email delivery for an issued token requires valid SMTP settings.

Start locally with:

```sh
VERSION=$(cat VERSION) JOBS_ADMIN_TOKEN=dev-token JOBS_SMTP_ADDR=smtp-relay.gmail.com:587 JOBS_SMTP_USERNAME=u JOBS_SMTP_PASSWORD=p docker compose up --build
```

## Development vs production stacks

The repository ships two compose files, and they never share state:

| | Production (`docker-compose.yml`) | Development (`docker-compose.dev.yml`) |
|---|---|---|
| Project name | `jobfinder` (from directory) | pinned: `jobfinder-dev` |
| Volume | `jobfinder_jobs-data` | pinned: `jobfinder-dev-data` |
| Web port | 8080 | 8090 |
| Admin port | 8081 | 8091 |
| SMTP | real credentials required | dummies (mail fails silently) |
| Admin token | required env | defaults to `dev-admin-token` |

Launch the dev stack with `./scripts/dev.sh up` (also `down`, `reset`, `status`,
`logs`). The fixed project and volume names mean the dev database persists
across launches from any directory; `./scripts/dev.sh reset` wipes only the dev
volume and can never touch production. Running production from a different
directory is safe for the same reason: its volume is always
`<project>_jobs-data`, and the pinned dev names cannot collide with it.

## External reverse proxy

The app services listen on port 8080 (web) and 8081 (admin). Production deploys should point a dedicated external nginx, or any TLS-terminating proxy, at those ports. TLS and certificates are entirely the proxy's concern.

Liveness is exposed at `/healthz` and intentionally does not check dependency readiness.
