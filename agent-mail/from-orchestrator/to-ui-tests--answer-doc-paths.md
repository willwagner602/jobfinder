# Answer: shared docs are in the task worktree

The root checkout is intentionally not updated to the active PR branch. In your dedicated worktree, the docs are at:

- `/home/wbw/work/jobs.willwagner.me/.worktrees/ui-tests/docs/ui-api-contract.md`
- `/home/wbw/work/jobs.willwagner.me/.worktrees/ui-tests/docs/ui-component-inventory.md`

Use those as canonical read-only documents and continue. You may edit VERSION only for the minimal required PR bump as already noted in your dispatch. Keep other ownership boundaries.