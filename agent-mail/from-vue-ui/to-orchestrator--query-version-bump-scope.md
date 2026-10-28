# Query: VERSION bump scope conflict

Task: `vue-ui`

## Context

The dispatch assigns ownership of:

- `frontend/**`
- `docs/ui-component-inventory.md`

It also says: "Do not edit Go files, deployment/container files, CI workflows, README, shared API contract, browser test/fixture/screenshot scripts, or UI spec. Do not edit files outside `frontend/**` and the named inventory doc."

However the acceptance section says: "All PRs require `VERSION` bump."

`VERSION` is at the repository root and is not included in the owned file list.

## Decision needed

May the `vue-ui` worker edit root `VERSION` solely to satisfy the PR version-bump requirement?

## Options

1. Allow `vue-ui` to bump `VERSION` in addition to its owned files.
2. Keep `VERSION` out of scope and have the orchestrator or another task apply the required bump.
3. Waive the version-bump requirement for this worker PR.

## Recommendation

Option 1 is simplest if each worker PR must independently satisfy repository PR policy. I will only edit `VERSION` for the required bump and keep all other changes inside `frontend/**` and `docs/ui-component-inventory.md`.

## Additional note

The dispatch asked me to read `/home/wbw/work/jobs.willwagner.me/docs/ui-api-contract.md`, but that absolute path is missing. The contract exists inside the assigned worktree at `/home/wbw/work/jobs.willwagner.me/.worktrees/vue-ui/docs/ui-api-contract.md`; I used the worktree copy for initial inspection.
