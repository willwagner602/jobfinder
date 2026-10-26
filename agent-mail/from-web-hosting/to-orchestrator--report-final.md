# Final report: web-hosting

## Built

- Implemented Go serving for built Vue SPA assets.
- Added `Server.Assets` and `NewWithAssets` so production/runtime can supply the built frontend directory while tests use fixture files.
- `cmd/web` now reads `SPA_ASSET_DIR`, defaulting to `frontend/dist` for local development.
- Production packaging contract coordinated from orchestrator message: packaging PR #4 copies the Vue `dist` contents to `/app/static` and owns setting `SPA_ASSET_DIR=/app/static` in the web container/runtime image. This task does not edit packaging-owned files.
- Routes:
  - `GET /assets/...` serves Vite-built files from the configured asset root.
  - User-facing SPA routes serve `index.html`: `/`, `/signup`, `/verify`, `/login`, `/onboarding`, `/jobs`, `/jobs/{id}`.
  - `/api/*`, `/healthz`, `/static/*`, legacy form POST routes, and unknown reserved/internal routes are not shadowed by the SPA fallback.
  - If `index.html` is absent, only known legacy user routes fall back to the old templates. This keeps current no-built-assets integration tests safe and does not broaden fallback to reserved paths.
- Added/updated tests for SPA route serving, asset serving, reserved-route behavior, configured runtime asset root, API ownership, and legacy fallback when no SPA index exists.
- Bumped `VERSION` from `0.1.3` to `0.1.4`.

## Tests run

- `gofmt -w cmd/web/main_test.go cmd/web/main.go internal/web/server.go internal/web/server_test.go internal/web/coverage_test.go` — passed.
- `go test ./cmd/web ./internal/web` — passed.
- `go test ./...` — passed.
- `go test -cover ./internal/web` — passed; coverage `90.3% of statements`.

## Git/PR

- Branch: `web-hosting`
- Commits:
  - `5c7f219 web-hosting: serve Vue SPA assets`
  - `27cba75 web-hosting: test configured SPA asset root`
- Pushed to `origin/web-hosting`.
- Opened PR to `master`: https://github.com/willwagner602/jobs.willwagner.me/pull/5

## Deviations / coordination notes

- No packaging files were edited, per orchestrator instruction. The exact production path is `/app/static` via `SPA_ASSET_DIR=/app/static`, owned by packaging PR #4.
- The local default remains `frontend/dist` for developer builds.
- I did not merge the PR.
