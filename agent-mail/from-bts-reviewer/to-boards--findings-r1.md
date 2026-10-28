# boards review findings — round 1

Scope reviewed: `internal/boards/**`, `cmd/board-search/**`, `data/job-boards.json` in worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/boards`, commit `2f932c44fec4d94759a474ca6752d7c8b3f4bf9f`, against `mvp-plan.md` Implementation Plan §2 and the Adzuna/SerpAPI limits/discovery-path acceptance criteria.

What I checked:
- Read the spec lines for Job Board Search, the discovery import boundary, job_history, and the named acceptance criteria.
- Read all owned files in full.
- Inspected the task diff for the owned paths.
- Attempted to run `go test ./internal/boards ./cmd/board-search`; could not run because `command -v go` returned no path in this environment.

## Findings

### important — discovered supported-ATS candidates are auto-activated

- Evidence: `mvp-plan.md:39` says the candidate import boundary feeds validation and that "discovered candidates are never auto-activated". `internal/boards/providers.go:117-120` derives `Supported` from the detected ATS for every provider URL, and `internal/boards/boards.go:191-192` maps any `Candidate{Supported: true}` directly to `schema.BoardStatusActive` before insert/update.
- Failure mode: a newly discovered Greenhouse/Lever/Ashby/Workday/Jobvite board from Adzuna or SerpAPI is inserted as `active` immediately, without any separate enablement/activation step. That violates the discovery-path requirement and can cause downstream scraping to treat unreviewed marketplace discoveries as enabled boards.
- Smallest correct fix: separate "adapter exists" from "board is active". Persist newly discovered candidates in a non-active state until an explicit activation path promotes them; if the current schema only has `active`/`unsupported`, add an appropriate pending/discovered status or otherwise avoid writing `active` for provider discoveries.

### important — default executable does not run the required Adzuna and SerpAPI sources

- Evidence: `mvp-plan.md:38` requires the separate daily executable to run the top-level `adzuna/serpapi/future data source searches`. In `cmd/board-search/main.go:24-27`, the executable defaults `-providers` to `fixture` and requires `-fixture` when that provider is enabled; `cmd/board-search/main.go:39-41` then runs only the providers built from that flag.
- Failure mode: invoking the new executable with its default configuration does not run Adzuna or SerpAPI at all; it fails unless a fixture path is supplied. A daily deployment using the executable defaults would perform no real board discovery, despite the task being the Adzuna/SerpAPI board-search executable.
- Smallest correct fix: make the deployable/default provider set the real daily sources (`adzuna,serpapi`, with credentials from env/flags), and keep fixture as an explicit test/offline mode.

## Verification note

The Go toolchain is unavailable in this review environment, so I could not execute the added tests. The attempted command was:

```sh
cd /home/wbw/work/jobs.willwagner.me/.worktrees/boards && command -v go && go test ./internal/boards ./cmd/board-search
```

`command -v go` produced no output, so the test command stopped before running tests.
