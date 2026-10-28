# Web hosting PR review — round 1

Scope reviewed: GitHub PR #5 (`web-hosting` -> `master`) at head `27cba75d86768243ef51a160c10ba7560533516f`, including the Vue ancestor commit carried by the branch. Focus was Go SPA asset serving/fallback and security: exact user-facing routes, `/api`, `/healthz`, and `/static` precedence, path traversal exposure, missing `index.html` behavior, configured `SPA_ASSET_DIR`, tests, and compatibility with the combined Vue branch contracts.

Spec checked: `ui-overhaul.md` frontend/backend architecture and acceptance criteria, especially serving built assets from the Go deployment unit, preserving Go authority/security controls, supporting deep links/refreshes, and not shadowing API routes, health checks, or admin routes (`ui-overhaul.md:19-25`, `ui-overhaul.md:91-99`).

Checks performed:

- Confirmed PR metadata with `gh pr view 5`: base `master`, head `web-hosting`, merge state `CLEAN`, head `27cba75d86768243ef51a160c10ba7560533516f`.
- Confirmed GitHub CI for the PR head completed: `test` check succeeded; `ghcr` check was skipped by workflow conditions.
- Reviewed changed Go files in full for the requested scope: `internal/web/server.go`, `internal/web/server_test.go`, `cmd/web/main.go`, and `cmd/web/main_test.go` from the PR head.
- Verified route precedence in `internal/web/server.go:162-188`: `/healthz`, `/static/`, `/assets/`, and all `/api` routes are registered explicitly before the SPA routes; `serveSPA` rejects non-SPA paths with 404.
- Verified the SPA route allowlist in `internal/web/server.go:241-249`: only `/`, `/signup`, `/verify`, `/login`, `/onboarding`, `/jobs`, and non-empty `/jobs/{...}` paths fall back to the SPA. Unknown public paths, `/admin`, `/api/unknown`, `/healthz/static`, and `/static/...` are not swallowed by the SPA fallback.
- Verified built asset serving in `internal/web/server.go:192-200`: `/assets/...` is served by `http.FileServer` from the configured asset filesystem. Vite production output references `/assets/index-*.js` and `/assets/index-*.css`, matching that route.
- Verified missing `index.html` behavior in `internal/web/server.go:203-239` and `internal/web/server_test.go:110-140`: when the SPA index is absent, the legacy Go-template user routes remain available instead of returning a broken blank route.
- Verified `SPA_ASSET_DIR` startup wiring in `cmd/web/main.go:17-35` and `cmd/web/main_test.go:85-141`: `run()` passes the configured asset directory to `web.NewWithAssets`, and `/login` serves the configured SPA index.
- Verified tests pin the main routing/security contracts: `internal/web/server_test.go:91-108` covers exact SPA routes and `/jobs/{id}` refresh; `internal/web/server_test.go:142-173` covers `/api`, `/healthz`, `/assets`, `/static`, `/admin`, and unknown-path precedence; API auth/CSRF/user-isolation coverage remains present in `internal/web/server_test.go:227-290` and `internal/web/server_test.go:641-657`.
- Ran `go test ./internal/web ./cmd/web` in a detached PR worktree: passed.
- Ran `go test ./...` in the detached PR worktree: passed.
- Ran `npm --prefix frontend ci && npm --prefix frontend test -- --run && npm --prefix frontend run build` in the detached PR worktree: passed after installing dependencies.
- Ran `git diff --check origin/master..origin/pr/5`: passed.

Notes:

- My first frontend test/build attempt failed because the fresh worktree did not have `node_modules` installed (`vitest: not found`). After `npm ci`, the same test/build path passed.
- `npm ci` reported dependency audit warnings/vulnerabilities, but I did not treat that as a blocking finding for this scoped review because the repository workflow does not appear to enforce `npm audit`, and I did not identify a concrete asset-serving or trust-boundary failure from those warnings.

Verdict: clean — no blocking findings remain in the reviewed scope.
