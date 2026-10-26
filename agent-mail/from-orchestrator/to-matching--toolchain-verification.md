# Matching integration failure — gofmt and tests now runnable

The Go toolchain is now available. `gofmt -l` reports dirty formatting:

```
internal/matching/matching.go
internal/matching/matching_test.go
```

(No compile errors in matching from vet; only gofmt.)

1. Run `gofmt -w internal/matching/matching.go internal/matching/matching_test.go internal/matching/llm.go internal/matching/llm_test.go cmd/job-match-search/main.go cmd/job-match-search/main_test.go` and commit.
2. Run `go test ./internal/matching ./cmd/job-match-search`; report exact results and any failures.
3. Commit on the matching branch. This is formatting/verification only — no behavior changes expected.
