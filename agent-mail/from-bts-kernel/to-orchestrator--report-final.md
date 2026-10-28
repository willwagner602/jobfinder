# Kernel report: discovered board status

## Built

- Added shared `schema.BoardStatusDiscovered` with value `discovered`.
- Bumped `schema.CurrentVersion` from 2 to 3.
- Added startup migration wiring for schema version 3.
- Added migration 3 that rebuilds `job_boards` with:
  - `status TEXT NOT NULL DEFAULT 'discovered'`
  - allowed statuses `discovered`, `active`, and `unsupported`
  - existing `board_id`, URLs, companies, ATS types, statuses, metadata, and timestamps copied through unchanged
  - `idx_job_boards_status` recreated after the constrained table rebuild
- Left historical migration 1 shape unchanged; fresh databases migrate sequentially through version 3.

## Contract surface exposed to leaf tasks

- Leaf code can use `schema.BoardStatusDiscovered` for supported boards found by discovery but not yet activated.
- Existing active boards remain `active`; existing unsupported boards remain `unsupported`.
- Inserts that omit `job_boards.status` now default to `discovered` after `EnsureCurrent` reaches schema version 3.
- Invalid statuses remain rejected by the SQLite check constraint.

## Tests/verification

- Added database test coverage for preserving existing active and unsupported board rows through upgrade.
- Added database test coverage for defaulting omitted board status to `discovered`.
- Added database test coverage accepting explicit `discovered` and rejecting invalid `pending`.
- Ran `git diff --check`: passed.
- Attempted documented `gofmt -w internal/schema/schema.go internal/db/db.go internal/db/db_test.go && go test ./...`: failed because `gofmt` is not installed/on PATH.
- Explicit toolchain probe: `command -v go` and `command -v gofmt` returned nothing; `go version` failed with `/usr/bin/bash: line 64: go: command not found` and exit 127.

## Commit

- `5bf7ee8 Add discovered job board status migration`

## Files changed

- `internal/schema/schema.go`
- `internal/db/db.go`
- `internal/db/db_test.go`

## Notes

- The requested resolver decision file `/home/wbw/work/jobs.willwagner.me/agent-mail/from-resolver/to-orchestrator--resolve-board-status.md` was not present in the mailbox. I proceeded from the orchestrator follow-up, which supplied the exact required semantics.
- No boards-owned files were modified.
