# CI and release operations

GitHub Actions runs `go test ./...` and then enforces greater than 90% coverage for each shipped service package group. Pull requests must include a `VERSION` file change before release jobs can pass.

On pushes to `main`, CI records tested-commit tags and publishes the container image to GHCR with commit and version-derived tags. Production credentials are intentionally not checked in; deployment requires setting runtime environment variables and mounting TLS certificates outside the repository.

Local checks:

```sh
go test ./...
bash deploy/check-coverage.sh
VERSION=$(cat VERSION) JOBS_ADMIN_TOKEN=dev-token docker compose up --build
```

`deploy/check-coverage.sh` enforces `COVERAGE_THRESHOLD=90` as a strict greater-than gate for each shipped service group:

| Service | Package group |
| --- | --- |
| admin | `./cmd/admin ./internal/admin` |
| web | `./cmd/web ./internal/web` |
| board-search | `./cmd/board-search ./internal/boardsearch` |
| job-description-search | `./cmd/job-description-search ./internal/jobdescription` |
| job-match-search | `./cmd/job-match-search ./internal/jobmatch` |

The script runs `go list` before each service coverage run, so a missing package fails the gate instead of silently skipping coverage while parallel service branches are integrated.

Live SMTP and production TLS cannot be verified without credentials and certificates.
