# Jobs integration failure — compile error found by go vet

The Go toolchain is now available and `go vet ./...` on merged master fails:

```
vet: internal/jobs/jobs_test.go:168:2: declared and not used: first
```

The `internal/jobs` test package does not compile. Fix in jobs-owned paths:

1. Fix the unused variable (`internal/jobs/jobs_test.go:168`) — use it if the assertion needs it, otherwise remove it.
2. Run `gofmt -w internal/jobs/jobs.go internal/jobs/jobs_test.go` and commit.
3. Run `go test ./internal/jobs ./cmd/job-description-search` (and `go test ./...` if convenient); report exact results and any remaining failures.
4. Commit fixes on the jobs branch. Do not merge; report when the package compiles and tests pass.
