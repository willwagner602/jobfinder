# Ops integration findings — round 2 (two deployment issues)

Round-2 integration review confirmed your web/CI compose fixes are correct. Two new issues were found, both in deployment surfaces you own.

**Finding A (blocker) — web container lacks its runtime template/static assets.**
Evidence: `deploy/web.Containerfile` builds `/usr/local/bin/jobs-web` from source but the runtime stage (`FROM debian:bookworm-slim`) copies only the binary. At startup the web server loads templates from disk (`templates/` via `ParseTemplates...` calls in `internal/web/server.go`) and serves CSS via `http.Dir("static")` at `/static/` (`internal/web/server.go:159`). In the deployed container those directories do not exist, so every page render fails (and static 404s) — i.e., the deployed web experience is broken even though local tests pass.

Fix (ops-owned files only): extend `deploy/web.Containerfile` so the final image contains the `templates/` and `static/` directories next to the binary's working directory (`WORKDIR /app; COPY --from=build /src/templates /app/templates` and same for `static`). No code changes; this is purely packaging.

**Finding B (major) — SMTP settings documented as optional but required at web startup.**
Evidence: `deploy/README.md:8` says `JOBS_SMTP_ADDR`/`USERNAME`/`PASSWORD` are "optional … If unset, token issuance still works" and `docker-compose.yml:11,32` default them to empty (`${VAR:-}`), but `cmd/web/main.go` fails startup when they are missing (web worker's report + `Server.New`/notifier construction path in `internal/web/smtp.go`). So the docs promise a mode the binary does not support; a deployed compose without SMTP values crashes the web service at boot.

Fix (ops-owned files only, keep code as-is): correct `deploy/README.md` to state the web service requires all three SMTP variables at startup (admin's mailer may genuinely be optional — check `internal/admin` usage separately and reflect that correctly rather than by assumption), and remove/raise the empty-default so omission is a loud compose-level error rather than an env that silently passes as "": e.g. use `${JOBS_SMTP_ADDR:?required}` in the web service stanza of `docker-compose.yml` (admin stanza can keep a default if that matches admin's real behavior). Keep the same variable names.

Verification: `JOBS_ADMIN_TOKEN=x docker compose config` still succeeds (with required vars passed), `bash -n deploy/*.sh`, `git diff --check`. If docker/podman is available locally, an image build of the web target proves the added directories exist: run it if cheap, else static-check paths. Commit on the ops branch, do not merge, report what changed per file.
