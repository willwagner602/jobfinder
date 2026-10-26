# Dispatch: web-hosting

Fix the production SPA serving and route fallback finding from PR #2 reviewer audit at `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-vue-ui--findings-r1.md` (finding 1), satisfying `ui-overhaul.md` §1. Use merged API contract at `docs/ui-api-contract.md`. The Vue build is currently in open PR #2 on branch `vue-ui`; read `frontend/index.html` and `frontend/package.json` from that branch/worktree to understand the asset path. Do not require this task to merge before PR #2; coordinate with orchestrator about rebasing/build-time assets.

## Goal

Serve the built Vue SPA from the existing Go web service, and have user-facing SPA routes/deep links resolve to its `index.html`, while preserving `/api/*`, `/healthz`, `/static/*` compatibility as decided, and not catching unknown reserved/internal/admin routes. No external runtime asset host or dev server.

## Owned files

- `internal/web/server.go`
- `internal/web/server_test.go`
- `cmd/web/main.go` only if necessary for runtime config
- `docs/ui-api-contract.md` only if route behavior/asset contract must be documented
- `VERSION` (minimal bump required by PR policy)

Do not edit frontend/**, test/fixture/screenshot files, CI/container deployment files, README/docs outside named contract, or admin code. Coordinate required asset root/build output with packaging worker. If SPA assets cannot be served until package output exists, define safe handler/config that is tested against fixture FS and query, don't ship broken assumption.

## Acceptance

- Tests show SPA routes (`/`, `/signup`, `/verify`, `/login`, `/onboarding`, `/jobs`, `/jobs/:id`) serve SPA index after integration, not old templates.
- API and health endpoints remain correctly dispatched and retain auth/CSRF behavior.
- Unknown route behavior is intentional and tested; no broad fallback shadows API/static/health/admin.
- Go tests for web pass, coverage remains >90%, gofmt and diff check pass.
- Open PR to master, do not merge; include VERSION bump.

## Worktree/mailbox

Worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/web-hosting`
Branch `web-hosting`
Report `/home/wbw/work/jobs.willwagner.me/agent-mail/from-web-hosting/to-orchestrator--report-final.md`
Queries `/home/wbw/work/jobs.willwagner.me/agent-mail/from-web-hosting/to-orchestrator--query-<topic>.md`
