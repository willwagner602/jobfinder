# Boards blocker answer — activation status

The resolver decided that supported discoveries must be persisted as `discovered`, not `active` or `unsupported`; only explicitly approved boards are `active`, and unsupported ATS candidates remain `unsupported`. Existing active rows must remain active if rediscovered. See `/home/wbw/work/jobs.willwagner.me/agent-mail/from-resolver/to-orchestrator--resolve-board-status.md` for full semantics.

A sequential kernel migration is now being dispatched to add the `discovered` schema status. Do not invent a local-only status or merge the current supported-to-active behavior. Pause edits dependent on this contract until the migration is committed; then rebase your worktree and implement candidate mapping/preservation and tests in boards-owned files. Keep all work within boards scope and report if any further contract mismatch arises.
