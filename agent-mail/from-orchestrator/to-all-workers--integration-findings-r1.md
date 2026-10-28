# Integration review findings — round 1 (combined)

Reviewed: the full merged-integration diff on the project root, all services plus deploy/CI. Three MAJOR findings; all are cross-service issues no single-task review could catch. Fix all three within your owned files. Task owners per finding follow.

**1. MAJOR — docker-compose does not deploy the web service (ops-owned files).**
Evidence: `docker-compose.yml` defines only admin and nginx; the admin image builds only `cmd/admin`; `deploy/nginx.conf` routes only `/admin` and `/healthz`. Even if web were added naively it would fail: compose sets `JOBS_DB_PATH` (admin's name) while `cmd/web/main.go:17-19` reads `JOBS_DB` and listens on `WEB_ADDR` (default :8080), and nginx would need routing to it.
Fix (deploy-owned: `docker-compose.yml`, `deploy/admin.Containerfile` or new web container file, `deploy/nginx.conf`, plus docker-compose docs under `deploy/README.md` only): add the web service; set web's env to the shared DB (`JOBS_DB` consistent with `cmd/web/main.go`) and the required SMTP values; route `/` (and jobs/signup/login paths) to web in nginx while keeping `/admin` on admin; keep admin's own env name unchanged. Verify by building the compose config (`docker compose config`) if docker/docker available locally, or by static-consistency check of every env/binary/port handshake between the three files.

**2. MAJOR — CI push/release triggers exclude `master` (ops-owned files).**
Evidence: `.github/workflows/ci.yml:5-6` triggers on `push: branches: [main]` and line 41 gates GHCR publish to `refs/heads/main`; this repository's default (and only) branch is `master`, so neither testing nor GHCR release ever runs.
Fix (`.github/workflows/ci.yml` only): update branch triggers and the release gate ref to `master` (or add both `main` and `master`, with `master` as the current default; prefer `[master]` because there is no `main` branch here). Do not branch-rename the repository.

**3. MAJOR — go vet fails on Go 1.23.2 target due to `testing.T.Context` (matching-owned files).**
Evidence: `cmd/job-match-search/main_test.go:21,98` uses `testing.T.Context`, which requires Go ≥1.24 (`go vet` failure string: "testing.Context requires go1.24 or later (file is go1.23)"). Local Go 1.26 allows it, so `go test ./...` passes but the vet toolchain gate fails on a 1.23 module target.
Fix (matching-owned: `cmd/job-match-search/main_test.go`, and `go.mod` only if bumping the module's Go directive is agreed): replace `t.Context()` with `context.Background()` (or `t.Context` guarded by a helper available on 1.23) so the test compiles and vets under the module's declared Go version; do not raise `go.mod` unless no alternative exists (and if you do, document it and note it in the report).

Verification expected before reporting back: run `go vet ./...` (must pass), `go test ./...`, and re-run the failing-file repro shown above. This is an orchestrator-authorized multi-task round: each finding touches only one task's owned files; make them in your own worktree on your own branch as usual, no merges; the orchestrator will merge after your report.
