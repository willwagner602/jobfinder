# Review findings for packaging r1 (GitHub PR #4)

Scope reviewed:
- PR #4 (`packaging` -> `master`) through GitHub PR workflow, not local diff.
- Spec: `ui-overhaul.md`, especially frontend/backend architecture and acceptance criteria for production serving, direct navigation/refresh, CI checks, and production deployment documentation.
- Changed PR files: CI workflow, `VERSION`, deployment docs/containerfile, and the included Vue frontend files.
- Overlap checked with PR #2 (`vue-ui` -> `master`): PR #4 includes the Vue frontend commit from PR #2 plus the packaging commit, so it is not an isolated packaging-only PR.

Checks performed:
- `gh pr view 4 --json ...` and `gh pr diff 4 --name-only` confirmed PR #4 targets `master`, contains commits `3a506ac` (Vue SPA frontend) and `d1d5525` (packaging), and includes the frontend tree.
- `gh pr view 2 --json ...` confirmed PR #2 separately carries the Vue frontend work, creating integration/ordering overlap.
- Inspected PR #4 patch for `.github/workflows/ci.yml`, `VERSION`, `deploy/README.md`, `deploy/web.Containerfile`, `frontend/package.json`, and `frontend/vite.config.ts`.
- Inspected current web routing in `internal/web/server.go`.
- Ran PR #4 checks locally from a detached worktree at PR head: `npm ci --prefix frontend`, `npm test --prefix frontend`, `npm run build --prefix frontend`, and `go test ./...` all passed.
- CI on PR #4 was verified passing before this report.

Verdict: findings — do not merge PR #4 as-is.

## Important: packaged Vue build is copied into the image but is not served as the SPA required by the spec

Evidence:
- `ui-overhaul.md:20` requires production to serve the built frontend assets from the existing Go web service or deployment unit, without a separate development server or third-party asset host.
- `ui-overhaul.md:24-25` require deep links and browser refreshes on SPA routes to resolve correctly without shadowing API routes, health checks, or admin routes.
- `ui-overhaul.md:91-92` make this an acceptance criterion: all seven pages are implemented in Vue, and the built SPA is served through the application deployment and works on direct navigation/refresh.
- PR #4 copies the build output into the runtime image at `/app/static`: `deploy/web.Containerfile:4871-4874` (`COPY --from=build /src/frontend/dist /app/static`). The docs also state this destination is the static asset directory served by the Go web service: `deploy/README.md:4839`.
- The Go web service route table still only serves `/static/` from a relative `static` directory and leaves the user page routes mapped to existing server-rendered handlers: `internal/web/server.go:156-171` has `GET /static/` using `http.FileServer(http.Dir("static"))`, then `GET /signup`, `GET /verify`, `GET /login`, `GET /`, `GET /onboarding`, `GET /jobs`, and `GET /jobs/` all route to Go template handlers.

Why this breaks the spec:
- The container contains Vite output under `/app/static`, but no route serves `/app/static/index.html` as the application entry point for `/`, `/signup`, `/verify`, `/login`, `/onboarding`, `/jobs`, or `/jobs/:matchId`.
- Existing page routes continue to render the old Go-template UI rather than the Vue SPA, so packaging the assets does not integrate the built SPA into production behavior.
- Browser refresh/deep link behavior for SPA routes is not implemented as a fallback to `index.html`; `/jobs/123` continues to hit the old Go job-detail handler, and routes not explicitly known to Go will not resolve to the SPA.
- Because PR #4 is based on PR #2 and includes the frontend commit, merging it directly to `master` would also integrate the whole Vue frontend in a packaging PR while still missing the server-side hosting changes needed to make those assets the production UI. That overlap should be resolved by either rebasing/sequencing after the frontend/web-hosting work or including the required hosting changes in the same integration PR.

Smallest correct fix:
- Add the web-hosting integration before merge: serve the Vite `index.html` and asset files from the Go web deployment for the user-facing SPA routes, add a fallback for SPA deep links/refreshes, and explicitly preserve `/api`, `/healthz`, `/static` asset paths as needed, plus admin routing/deployment behavior.
- Update tests/acceptance checks to prove direct navigation/refresh for the required SPA routes returns the Vue entry point without breaking health/API/admin routes.
- Align the PR dependency with PR #2/web-hosting work: either make PR #4 target/include the prerequisite frontend hosting branch deliberately, or rebase it so packaging changes are reviewed after the frontend and web-hosting changes have landed.

Notes:
- `VERSION` is bumped from `0.1.3` to `0.1.4`, satisfying the repository's PR version-bump workflow requirement.
- CI behavior in the patch adds Node 22 setup, `npm ci`, `npm test`, and `npm run build` for frontend checks; those commands passed locally at PR #4 head, and PR CI was verified passing.
- The image build target switch from `deploy/admin.Containerfile` to `deploy/web.Containerfile` appears intentional for publishing the web runtime image, but it does not by itself make the Vue app reachable.
