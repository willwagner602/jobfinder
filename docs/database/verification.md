# Shared database verification

The shared database kernel is a Go module with SQLite-backed migrations and contract tests.

## Local checks

Run from the repository root:

```sh
go test ./...
```

This exercises:

- fresh database initialization to the published schema version
- upgrade from the checked-in previous-version fixture at `internal/db/testdata/schema_v1.sql`
- schema version consistency between code and database state
- canonical URL listing identity, immutable content versions, normalized content hashing, and key/check constraints

## Dependency note

The kernel uses `github.com/mattn/go-sqlite3` because Go's standard library exposes the `database/sql` abstraction but does not ship a SQLite driver. No other runtime dependency is required.
