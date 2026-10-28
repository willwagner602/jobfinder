# Kernel follow-up: board discovery status

The board activation query is resolved; see `/home/wbw/work/jobs.willwagner.me/agent-mail/from-resolver/to-orchestrator--resolve-board-status.md` for rationale and exact semantics.

Implement a sequential schema migration adding `discovered` to `job_boards.status`; make the status default safe (discovered), preserving all existing row values/IDs (especially existing active rows). Rebuild the constrained table and recreate its index as needed. Add migration coverage including acceptance of `discovered` and rejection of invalid values, and preserve existing active rows through upgrades. Update shared schema constants/current version and startup migration wiring. Stay in kernel-owned files, commit the change in the kernel worktree/branch, and report verification limitations precisely. This is a kernel follow-up because the board worker cannot safely change the shared contract or DB constraint.
