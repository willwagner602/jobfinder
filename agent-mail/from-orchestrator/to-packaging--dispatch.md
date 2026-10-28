# Dispatch: packaging

Implement production frontend packaging and CI integration for `/home/wbw/work/jobs.willwagner.me/ui-overhaul.md` §§1 and 5–7. Kernel PR #1 is merged at master commit `8419179` with Go JSON API and asset-independent route contract. Read the spec and inspect current CI/container setup.

## Contract assumptions

- Vue source will be in `frontend/`.
- Production frontend build command will be `npm ci && npm run build` with output `frontend/dist/`.
- Existing Go web service will serve the built asset bundle; SPA route fallback is being handled by Go asset-serving owner after its route contract. Do not modify Go files.
- Browser test worker will provide test scripts later; coordinate with orchestrator before editing test runner config.

## Owned files

- `deploy/web.Containerfile`
- `.github/workflows/ci.yml`
- `deploy/README.md` (update deployment/build docs)
- `VERSION` (required CI gate; bump once per PR)

Do not edit `README.md`, Go source/tests, frontend sources, fixture/test/screenshot scripts, shared API docs, or spec. You may run existing tests/builds but must not edit files outside your scope.

## Work

- Build Vue assets reproducibly in a Node builder stage, then Go binary in Go build stage; include built assets in runtime image as needed by Go serving contract, without shipping Node or dev server in runtime.
- Add CI steps for reproducible frontend dependency install/build and frontend-focused checks available from package scripts; coordinate test commands with orchestrator, don't assume future browser command prematurely.
- Preserve existing Go/integration/coverage CI and VERSION gate. Ensure PR passes `test` check; bump VERSION (small patch) in this PR.
- Update deployment docs with accurate local/production frontend build behavior. No production credentials in docs.

## Worktree and mailbox

Worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/packaging`
Branch/task id `packaging`
Report `/home/wbw/work/jobs.willwagner.me/agent-mail/from-packaging/to-orchestrator--report-final.md`
Queries `/home/wbw/work/jobs.willwagner.me/agent-mail/from-packaging/to-orchestrator--query-<topic>.md`

## Acceptance

- Validate `docker build -f deploy/web.Containerfile .` (or report environmental reason if Docker unavailable).
- Validate frontend build once source lands; do not invent placeholders.
- Existing Go CI remains intact; all checks pass on PR.
- No secrets or local .env data enter commits.
- Commit/push branch `packaging`, open PR targeting master, do not merge. Report exact commands and results.