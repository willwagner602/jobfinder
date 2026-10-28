# PR #4 packaging review r1b

Verdict: findings — one blocking packaging/runtime path contract issue remains.

Scope reviewed:
- `.github/workflows/ci.yml`
- `deploy/web.Containerfile`
- `deploy/README.md`
- `VERSION`
- Runtime asset directory agreement with the current Go web-hosting task
- PR ancestry/integration target safety for the Vue ancestor content included in PR #4

Checks run on `origin/pr-4` (`d1d5525410352af606a1204ac3f8f5e2cc606253`):
- `npm --prefix frontend ci`
- `npm --prefix frontend test -- --run`
- `npm --prefix frontend run build`
- `go test ./...`
- `bash deploy/check-coverage.sh`
- `go test ./internal/itests/...`

GitHub PR state checked with `gh pr view 4`: base `master`, head `packaging`, merge state `CLEAN`, CI `test` success, `ghcr` skipped for PR as expected.

## Finding

1. **Important — The production image does not set the SPA asset directory expected by the Go web-hosting task.**

   Evidence:
   - `deploy/web.Containerfile:18-21` sets `WORKDIR /app` and copies the frontend build to `/app/static`.
   - `deploy/README.md:15` documents `/app/static` as the runtime static asset directory.
   - Current web-hosting branch `cmd/web/main.go:20` reads `SPA_ASSET_DIR` with default `frontend/dist` for local development, and `internal/web/server.go:136` / `:197-200` serves SPA files from that configured asset filesystem.
   - Coordination mail `agent-mail/from-orchestrator/to-packaging--asset-path.md:3` asks packaging to set `SPA_ASSET_DIR=/app/static` in the web runtime image or Compose environment.

   Why this matters:
   - The spec requires the built SPA to be served through the application deployment and production not to depend on a dev server (`ui-overhaul.md:19-24`, `ui-overhaul.md:91-99`).
   - After the Go web-hosting task lands, the container will contain the built files at `/app/static`, but the web process will look at the local-development default `frontend/dist` unless packaging provides the production env var. That breaks the agreed runtime contract and risks falling back to legacy/404 behavior instead of serving the packaged SPA.

   Smallest correct fix:
   - In the packaging-owned runtime configuration, set `SPA_ASSET_DIR=/app/static` for the web container/image, e.g. `ENV SPA_ASSET_DIR=/app/static` in `deploy/web.Containerfile` or the equivalent Compose environment if that is the chosen production runtime source.

## Integration target note

PR #4 is based on Vue ancestor commit `3a506ac`, while the current Vue PR #2 head is `96d4a89` and contains an additional frontend fix (`frontend/index.html` plus `frontend/src/themeBootstrap.test.ts`). The PR #4 merge state against `master` is clean, but the stacked integration target is stale relative to the current Vue branch. Rebuild/rebase PR #4 on the current Vue head, or merge after PR #2 and refresh PR #4, so the packaging branch does not carry older frontend content.
