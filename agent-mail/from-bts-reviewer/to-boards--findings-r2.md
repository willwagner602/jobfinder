# boards review findings — round 2

Scope reviewed: `internal/boards/**`, `cmd/board-search/**`, `data/job-boards.json` in worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/boards`, commit `fde6c952ab0eb08fd614c12999b0e3d0af51f3ab`, compared with base `70bab3e`, rebased onto kernel migration `5bf7ee8`.

What I checked:
- Read `mvp-plan.md` Implementation Plan §2 and relevant acceptance criteria lines 37-39 and 73-78.
- Read the worker final report at `/home/wbw/work/jobs.willwagner.me/agent-mail/from-boards/to-orchestrator--report-final.md`.
- Read all owned files in full: `internal/boards/boards.go`, `internal/boards/providers.go`, `internal/boards/boards_test.go`, `internal/boards/providers_test.go`, `cmd/board-search/main.go`, `cmd/board-search/main_test.go`, and `data/job-boards.json`.
- Inspected the owned diff against `70bab3e`.
- Attempted to run `go test ./internal/boards ./cmd/board-search`; could not run because `command -v go` returned no path in this environment.

Resolved from prior review:
- Supported candidates now persist as `discovered`, not `active`: `internal/boards/boards.go:191-192` maps `Candidate.Supported` to `schema.BoardStatusDiscovered`.
- Existing active rows remain active on rediscovery: `internal/boards/boards.go:174` uses `CASE WHEN status = 'active' THEN status ELSE ? END`.
- Unsupported candidates are retained as rows with `unsupported` status: `internal/boards/boards.go:191-192` and tests at `internal/boards/boards_test.go:29-55`.

## Findings

### important — default executable does not run the required Adzuna and SerpAPI sources

- Evidence: `mvp-plan.md:38` requires the separate daily executable to run the top-level `adzuna/serpapi/future data source searches`, and the acceptance criteria require pulling from Adzuna and SerpAPI within the stated limits at `mvp-plan.md:76`. In `cmd/board-search/main.go:26`, the executable defaults `-providers` to `fixture`; `cmd/board-search/main.go:53-55` then requires `-fixture` and appends only the fixture provider for that default path, while Adzuna and SerpAPI are added only when explicitly named at `cmd/board-search/main.go:56-59`.
- Failure mode: invoking the deployable daily executable with its default configuration does not run Adzuna or SerpAPI at all; it fails unless a fixture path is supplied. A scheduled daily run using defaults therefore performs no real board discovery, despite the spec requiring this executable to run the top-level Adzuna/SerpAPI searches.
- Smallest correct fix: make the executable's default provider set the real daily sources (`adzuna,serpapi`, using env/flag credentials), and keep `fixture` as an explicit test/offline provider mode.

## Verification note

The Go toolchain is unavailable in this review environment, so I could not execute the added tests. The attempted command was:

```sh
cd /home/wbw/work/jobs.willwagner.me/.worktrees/boards && command -v go && go test ./internal/boards ./cmd/board-search
```

`command -v go` produced no output, so the command exited before running tests.
