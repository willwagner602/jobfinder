# Authorized change: fix coverage-group paths in deploy/check-coverage.sh

The gate currently expects package groups that don't match the real module layout, so three of five groups can't even be measured:

```sh
  [board-search]="./cmd/board-search ./internal/boardsearch"        # no such package
  [job-description-search]="./cmd/job-description-search ./internal/jobdescription"  # no such package
  [job-match-search]="./cmd/job-match-search ./internal/jobmatch"   # no such package
```

Actual production packages from the merged branches are:

- board search: `./cmd/board-search` and `./internal/boards`
- job description search: `./cmd/job-description-search` and `./internal/jobs`
- job match search: `./cmd/job-match-search` and `./internal/matching`
- web / admin already match (`./internal/web`, `./internal/admin`, etc.)

Authorized change, ops-owned file only:

1. In `deploy/check-coverage.sh`, change those three group definitions to point at the real package paths shown above.
2. Do not touch the gate's threshold logic, its missing-package failure semantics, or any other file.
3. Also update the mapping table in `docs/operations/ci-release.md` to reflect the corrected paths, so docs and script stay in sync.

Why: this is a factual correction to make the per-service coverage gate enforceable over the real codebase; renaming worker packages would break imports across merged services. Stay in ops-owned files, commit on the ops branch, do not merge yourself; report exact script output on your branch for admin/web only (others can only be confirmed at integration).
