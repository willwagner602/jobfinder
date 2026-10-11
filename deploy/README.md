# Container deployment

The web and admin services share the SQLite database volume. Public job, signup, and login routes are served by web; `/admin` and `/healthz` are served by admin.

Required runtime configuration:

- `JOBS_ADMIN_TOKEN`: bearer token required for `/admin` pages.
- `JOBS_SMTP_ADDR`, `JOBS_SMTP_USERNAME`, `JOBS_SMTP_PASSWORD`: required by the web service at startup for the jobs@willwagner.me outbox. Set all three before starting the deployment. The admin service can start without SMTP configured, but email delivery for an issued token requires valid SMTP settings.

Start locally with:

```sh
VERSION=$(cat VERSION) JOBS_ADMIN_TOKEN=dev-token JOBS_SMTP_ADDR=smtp-relay.gmail.com:587 JOBS_SMTP_USERNAME=u JOBS_SMTP_PASSWORD=p docker compose up --build
```

## External reverse proxy

The app services listen on port 8080 (web) and 8081 (admin). Production deploys should point a dedicated external nginx, or any TLS-terminating proxy, at those ports. TLS and certificates are entirely the proxy's concern.

Liveness is exposed at `/healthz` and intentionally does not check dependency readiness.
