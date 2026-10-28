# Kernel integration — gofmt one test file

The Go toolchain is now available. `gofmt -l` reports:

```
internal/db/db_test.go
```

(No compile errors from vet in `internal/db`; only formatting.)

1. Run `gofmt -w internal/db/db_test.go internal/db/db.go internal/schema/schema.go internal/schema/schema_test.go` and commit.
2. Run `go test ./internal/db ./internal/schema ./cmd/orchestrator`; report exact results.
3. Commit on the kernel branch. Formatting/verification only.
