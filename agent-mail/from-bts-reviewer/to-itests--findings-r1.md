# Review verdict — integration testing framework r1

Verdict: clean

Scope reviewed:

- Spec section: `/home/wbw/work/jobs.willwagner.me/admin-interface-spec.md`, lines 19-22 (`Integration testing framework`).
- Worktree: `/home/wbw/work/jobs.willwagner.me/.worktrees/itests` at commit `39687e8` against base `87e1320`.
- Worker report: `/home/wbw/work/jobs.willwagner.me/agent-mail/from-itests/to-orchestrator--report-final.md`.
- Changed files from `git diff --name-only 87e1320..39687e8`:
  - `.github/workflows/ci.yml`
  - `internal/itests/README.md`
  - `internal/itests/integration_test.go`

What I checked:

- Harness shape: `internal/itests/integration_test.go:180-221` builds `cmd/web` and `cmd/admin`, creates a per-test `testing.T.TempDir` SQLite DB, allocates per-test localhost ports, starts both binaries with `exec.Command`, and waits on their real `/healthz` endpoints. `internal/itests/integration_test.go:392-399` uses stdlib `net/http` with a `cookiejar`; I found no browser/UI automation framework or in-process handler driving in the integration package.
- Flow coverage: the seven named flows map to tests at `internal/itests/integration_test.go:37`, `:70`, `:88`, `:103`, `:119`, `:137`, and `:149`. The implemented flows assert statuses, redirects, or response body content that can fail. I did not find a test that asserts nothing meaningful.
- Documented skips: `internal/itests/integration_test.go:131-134` skips the tombstoned same-email signup rejection only when current signup succeeds; `internal/itests/integration_test.go:162-165` skips signup-disabled enforcement only when current signup succeeds. The messages match current behavior: web signup inserts with `ON CONFLICT(email) WHERE status <> 'tombstoned'` at `internal/web/server.go:209`, the schema index excludes tombstoned users at `internal/db/db.go:308`, and `signupPost` does not consult `site_config.signups_disabled` before redirecting at `internal/web/server.go:189-219`.
- CI scope: `.github/workflows/ci.yml` has one added step only, `Integration tests`, running `go test ./internal/itests/...`; the workflow was not rewritten.
- README convention doc: `internal/itests/README.md` exists and documents the subprocess test harness, current flows, how to add future UI-flow tests, local execution, and CI convention.

Local checks run from `/home/wbw/work/jobs.willwagner.me/.worktrees/itests`:

- `go test ./internal/itests/...` — passed.
- `go test ./...` — passed.
- `bash deploy/check-coverage.sh` — passed.
- `gofmt -l .` — passed with no output.
- `go vet ./...` — passed with no output.
- Additional confirmation: `go test -v ./internal/itests/...` — passed, with the two intended skips reported at `internal/itests/integration_test.go:132` and `internal/itests/integration_test.go:163`.

No blocking findings remain for this leaf.
