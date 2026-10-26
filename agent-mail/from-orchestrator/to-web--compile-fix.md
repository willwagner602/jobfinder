# Web integration failure — compile error found by go vet

The Go toolchain is now available and `go vet ./...` on merged master fails:

```
internal/web/server.go:166:47: cannot use s.logoutPost (value of type
func(w http.ResponseWriter, r *http.Request)) as
func(http.ResponseWriter, *http.Request, *user) value in argument to s.requireAuth
```

`internal/web` does not compile. Fix in web-owned paths:

1. Correct the handler signature (or the handler) so `logoutPost` matches `requireAuth`'s expected handler type; make sure logout still requires authentication and works end-to-end.
2. Run `gofmt -w cmd/web/main.go internal/web/server.go internal/web/server_test.go internal/web/smtp.go` and commit.
3. Run `go test ./...` and any scoped tests that now pass; report exact results and any remaining failures.
4. Commit fixes on the web branch. Do not merge; report when the package compiles and tests pass.
