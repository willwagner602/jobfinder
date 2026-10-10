# Container deployment

This deployment runs the admin service behind nginx for `jobs.willwagner.me`.

Required runtime configuration:

- `JOBS_ADMIN_TOKEN`: bearer token required for `/admin` pages.
- `JOBS_SMTP_ADDR`, `JOBS_SMTP_USERNAME`, `JOBS_SMTP_PASSWORD`: optional SMTP settings for the jobs@willwagner.me outbox. If unset, token issuance still works but email sending is unavailable.
- `deploy/certs/`: mount TLS assets here when production certificates are available. The checked-in nginx config is HTTP-only so local and CI validation do not require secrets.

Start locally with:

```sh
VERSION=$(cat VERSION) JOBS_ADMIN_TOKEN=dev-token docker compose up --build
```

Liveness is exposed at `/healthz` and intentionally does not check dependency readiness.
