Scope reviewed: GitHub PR #2 (`vue-ui: add Vue SPA frontend`) against `ui-overhaul.md` §§1–3: Vue SPA architecture, API contract use, auth/CSRF, routing, accessibility, theme behavior, page states, and UI correctness.

Checks run:
- `cd frontend && npm ci && npm test` — passed (5 tests); npm reported 6 dependency audit vulnerabilities.
- `cd frontend && npm run build` — passed.
- `go test ./internal/web` — passed.

Verdict: findings — blocking issues remain.

1. **Important — The built SPA is not served by the Go web service, and direct SPA routes still hit the old Go-template handlers.** `internal/web/server.go:157-183` registers `/static/`, `/api/*`, and the existing template handlers for `/`, `/signup`, `/verify`, `/login`, `/onboarding`, `/jobs`, and `/jobs/`; there is no handler for `frontend/dist`/`index.html` and no SPA history fallback. Meanwhile the Vue router defines those same user routes (`frontend/src/main.ts:14-23`). This means production still serves the old pages on `/signup`, `/login`, `/jobs`, etc., and the new Vue app is only a standalone Vite artifact unless someone manually serves `frontend/dist` elsewhere. Section 1 requires the SPA assets to be served from the existing Go service/deployment, production not to depend on a separate dev server, and deep links/refreshes to resolve without shadowing API, health, or admin routes. Smallest correct fix: wire the built frontend assets into the web service, serve `index.html` for user-facing SPA routes and safe fallback paths, and keep `/api/*`, `/healthz`, `/static/*`/assets, and admin routes excluded and tested.

2. **Important — The pre-render theme bootstrap ignores the persisted manual theme choice.** The Vue theme module persists the manual choice under `jobs-theme` (`frontend/src/theme.ts:4`, `frontend/src/theme.ts:27-30`), but `frontend/index.html:6-10` reads `localStorage.getItem('theme')` before rendering. A user who previously selected a theme will initially get the OS-preferred class instead of their saved choice until Vue mounts, causing the wrong theme to flash and violating section 3’s requirements to persist a manual choice and apply the selected theme before rendering where practical. Smallest correct fix: make the inline bootstrap use the same storage key/semantics as `theme.ts` (or share a generated constant) and cover it with a test or build-time check.
