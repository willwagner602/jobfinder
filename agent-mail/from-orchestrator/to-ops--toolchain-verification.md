# Admin/ops integration failure — gofmt found dirty file

The Go toolchain is now available. `gofmt -l` reports:

```
internal/admin/admin.go
```

1. Run `gofmt -w internal/admin/admin.go internal/admin/server.go internal/admin/server_test.go internal/admin/admin_test.go cmd/admin/main.go` and commit.
2. Run `go test ./internal/admin ./cmd/admin` and `bash deploy/check-coverage.sh` if all service packages exist on your branch — they may not, since other service branches merge independently; report exactly what runs and what cannot (missing packages are expected until integration).
3. Commit on the ops branch. Formatting/verification only — no behavior changes expected.
