# Container deployment

This deployment runs the web and admin services behind nginx for `jobs.willwagner.me`. Public job, signup, and login routes go to the web service; `/admin` and `/healthz` go to admin. Both services use the shared SQLite database volume.

Required runtime configuration:

- `JOBS_ADMIN_TOKEN`: bearer token required for `/admin` pages.
- `JOBS_SMTP_ADDR`, `JOBS_SMTP_USERNAME`, `JOBS_SMTP_PASSWORD`: required by the web service at startup for the jobs@willwagner.me outbox. Set all three before starting the deployment. The admin service can start without SMTP configured, but email delivery for an issued token requires valid SMTP settings.
- `deploy/certs/`: mount TLS assets here when production certificates are available. The checked-in nginx config is HTTP-only so local and CI validation do not require secrets.

Start locally with:

```sh
VERSION=$(cat VERSION) JOBS_ADMIN_TOKEN=dev-token docker compose up --build
```

Liveness is exposed at `/healthz` and intentionally does not check dependency readiness.
