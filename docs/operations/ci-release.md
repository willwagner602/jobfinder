# CI and release operations

GitHub Actions runs `go test ./...` with a repository-level coverage gate. Pull requests must include a `VERSION` file change before release jobs can pass.

On pushes to `main`, CI records tested-commit tags and publishes the container image to GHCR with commit and version-derived tags. Production credentials are intentionally not checked in; deployment requires setting runtime environment variables and mounting TLS certificates outside the repository.

Local checks:

```sh
go test ./...
go test ./internal/admin -coverprofile=coverage.out
go tool cover -func=coverage.out
VERSION=$(cat VERSION) JOBS_ADMIN_TOKEN=dev-token docker compose up --build
```

Live SMTP and production TLS cannot be verified without credentials and certificates.
