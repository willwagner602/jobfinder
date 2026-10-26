# Query: frontend source missing for packaging task

## Context

Task `packaging` is assigned production frontend packaging and CI integration for `ui-overhaul.md` §§1 and 5–7. The dispatch contract says:

- Vue source will be in `frontend/`.
- Production frontend build command will be `npm ci && npm run build` with output `frontend/dist/`.
- Owned files are `deploy/web.Containerfile`, `.github/workflows/ci.yml`, `deploy/README.md`, and `VERSION`.

I inspected the packaging worktree at `/home/wbw/work/jobs.willwagner.me/.worktrees/packaging`. There is currently no `frontend/` directory, `package.json`, lockfile, or package scripts available to inspect or run.

## Blocker

I cannot safely implement or validate the required Node builder stage and CI frontend install/build steps without the frontend source and lockfile, because doing so would require guessing package manager state, package scripts beyond `build`, and the exact files that must be copied into the runtime image. The acceptance criteria also require validating the frontend build once source lands, and the dispatch explicitly says not to invent placeholders.

## Options

1. Wait until the frontend task lands `frontend/` with its lockfile and package scripts, then resume packaging and implement/test against the real files.
2. Author CI/container logic now with hard assumptions about `frontend/package-lock.json` and `npm run build`, but this risks breaking once the actual frontend structure lands and would be unvalidated.
3. Provide an explicit orchestrator-approved interim contract for the exact frontend files/scripts and whether CI should fail or skip while `frontend/` is absent.

## Recommendation

Resume `packaging` after the frontend source and lockfile are present, or provide an explicit interim contract. I recommend option 1 so the containerfile and CI can be reproducible and tested against the actual frontend build output.
