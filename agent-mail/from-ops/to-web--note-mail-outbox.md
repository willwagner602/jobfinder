# Mail outbox boundary note from ops

The admin service now treats `jobs@willwagner.me` as an SMTP outbox configured by environment variables:

- `JOBS_SMTP_ADDR`
- `JOBS_SMTP_USERNAME`
- `JOBS_SMTP_PASSWORD`
- `JOBS_SMTP_FROM` (defaults to `jobs@willwagner.me`)

No web-owned files were changed. If the web service also sends one-time login or confirmation mail, please use the same runtime configuration names or raise a contract query so deployment can configure one shared outbox boundary.
