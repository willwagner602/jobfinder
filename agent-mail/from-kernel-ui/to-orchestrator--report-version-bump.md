# kernel-ui VERSION bump follow-up

## Result

Updated PR #1 on branch `kernel-ui` and did not merge.

PR: https://github.com/willwagner602/jobs.willwagner.me/pull/1

## Version change

- `VERSION`: `0.1.1` -> `0.1.2`

## Commits pushed after the original kernel report

- `87a5a79 Bump version for UI API PR`
  - Changed root `VERSION` from `0.1.1` to `0.1.2` as requested.
- `c341fd2 Cover UI API error branches`
  - Added focused tests in the originally allowed `internal/web/coverage_test.go` after the CI rerun showed the VERSION gate was cleared but web coverage was below threshold.

Current PR head: `c341fd285528a1a8afd916aeaebd9f623c9f2f9d`

## Local verification

Commands run from `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel-ui`:

```sh
go test -coverprofile=/tmp/web.out ./internal/web
go tool cover -func=/tmp/web.out | tail -5
go test ./internal/web ./cmd/web
```

Results:

```text
ok  	jobs.willwagner.me/internal/web	2.491s	coverage: 91.1% of statements
total:						(statements)			91.1%
ok  	jobs.willwagner.me/internal/web	2.319s
ok  	jobs.willwagner.me/cmd/web	(cached)
```

## CI status

Final `gh pr checks 1 --watch=false` result on run `38187495761`:

```text
ghcr	skipping	0	https://github.com/willwagner602/jobs.willwagner.me/actions/runs/38187495761/job/114613008759
test	pass	37s	https://github.com/willwagner602/jobs.willwagner.me/actions/runs/38187495761/job/114612887157
```

Earlier observation: immediately after `87a5a79`, CI reran and failed on web coverage (`81.0% is not greater than 90.0%`), so I added the focused coverage-test commit within the original allowed file scope.

## Worktree status

`git status --short --branch`:

```text
## kernel-ui...origin/kernel-ui
```
