# Review findings — admin-ui r1

Scope reviewed:
- Spec bullets for admin user management and overall admin UI/menu styling in `admin-interface-spec.md`.
- Worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/admin-ui`, branch `admin-ui`, commit `7877166` vs base `87e1320`.
- Owned files read: `internal/admin/server.go`, `internal/admin/server_test.go`.
- Changed-file scope checked with `git diff --name-only 87e1320..7877166`: only `internal/admin/server.go` and `internal/admin/server_test.go` changed.

Checks run from the worktree:
- `gofmt -l internal/admin/server.go internal/admin/server_test.go` — no output.
- `go test ./...` — passed.
- `go vet ./...` — passed.
- `bash deploy/check-coverage.sh` — failed at the admin group: `admin coverage 86.2% is not greater than 90.0%`.

## Important findings

1. **Unauthenticated GET handlers were registered for admin user-action paths.**

   Evidence: `internal/admin/server.go:84-87` registers `GET /admin/users/approve`, `GET /admin/users/suspend`, `GET /admin/users/tombstone`, and `GET /admin/users/untombstone` directly to `methodNotAllowed`, while the authenticated POST handlers are wrapped with `s.requireAdmin` at `internal/admin/server.go:88-91`. `methodNotAllowed` itself only returns 405 and performs no authorization at `internal/admin/server.go:106-108`.

   Why this violates the requirement: the review instructions require every new route to remain behind admin token auth, with `401` without a valid bearer. The tombstone/untombstone GET registrations are new admin routes/surfaces and currently disclose a 405 response without authentication instead of requiring admin auth first.

   Smallest correct fix: wrap the GET method-blocker registrations for admin-only user action paths in `s.requireAdmin`, or remove the explicit GET registrations if the mux can reject unsupported methods without creating an unauthenticated admin surface. Add a test that unauthenticated requests to every new admin path, including wrong-method paths, return 401.

2. **The requested admin coverage gate fails.**

   Evidence: `deploy/check-coverage.sh:4` sets the default coverage threshold to 90, and `deploy/check-coverage.sh:21` fails when total coverage is not greater than that threshold. Running the requested command in the review worktree produced `admin coverage 86.2% is not greater than 90.0%` and exited non-zero.

   Why this violates the requirement: the review instructions explicitly require running `bash deploy/check-coverage.sh` for the admin group and treating the acceptance checks as part of the review. This implementation does not satisfy that gate.

   Smallest correct fix: add meaningful admin tests for uncovered new/error paths until the admin group exceeds the threshold; do not lower or bypass the gate.
