# Query: hard-coded schema version test outside kernel edit scope

Kernel task K bumped `schema.CurrentVersion` from 3 to 4 and added the v4 migration/store contracts. Focused checks pass:

- `gofmt -w internal/schema/schema.go internal/db/db.go internal/db/db_test.go internal/admin/admin.go internal/admin/admin_test.go`
- `go test ./internal/db ./internal/admin`

When running the required full check `go test ./... && bash deploy/check-coverage.sh && test -z "$(gofmt -l internal/schema/schema.go internal/db/db.go internal/db/db_test.go internal/admin/admin.go internal/admin/admin_test.go)" && go vet ./...`, the first command fails in an unowned path:

```text
--- FAIL: TestRunAppliesSchemaAndServesRoutes (0.10s)
    main_test.go:107: schema version = 4, want 3 after run() startup
FAIL	jobs.willwagner.me/cmd/admin	1.188s
```

The failing assertion is in `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel2/cmd/admin/main_test.go`:

```go
if version != 3 {
    t.Fatalf("schema version = %d, want 3 after run() startup", version)
}
```

## Options

1. Authorize kernel to update this test to compare against `schema.CurrentVersion` (or literal 4) even though `cmd/admin/**` was not in the owned file list.
2. Keep kernel strictly scoped and let a later/parent task update the unowned test; kernel would then report full-suite failure as an expected cross-scope follow-up.

## Recommendation

Authorize the minimal test-only edit in `cmd/admin/main_test.go` to use `schema.CurrentVersion`. It is directly coupled to the schema-version contract and does not implement leaf behavior, routes, templates, or deployment changes.
