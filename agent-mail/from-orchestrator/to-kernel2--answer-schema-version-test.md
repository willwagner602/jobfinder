# Answer to kernel query — cmd/admin schema-version test

The resolver's position is confirmed: the correct fix is option 1, applied as a minimal, test-only change.

Authorize updating `/home/wbw/work/jobs.willwagner.me/.worktrees/kernel2/cmd/admin/main_test.go` so the assertion compares against `schema.CurrentVersion` (preferred — it removes the hard-coded number entirely and won't break on future bumps) instead of the literal `3`.

Scope of the authorization, and nothing beyond it:
- Touch **only** the assertion line and its error message in `TestRunAppliesSchemaAndServesRoutes`.
- If `schema` isn't already imported in that file, add the import as part of the same edit.
- Do **not** modify any other test, add new tests, change production code under `cmd/admin`, or open any file outside `cmd/admin/main_test.go`.

Rationale: this test is a direct consumer of the contract you own (`schema.CurrentVersion`); leaving it broken just to preserve a literal-scope rule would defeat the purpose of the kernel's migration while leaving the full-suite check red for the next worker. The minimal test-only drift-follower edit is the smallest change that restores "full checks pass" without touching leaf-owned surfaces.

After applying it, re-run the full chain (`go test ./... && bash deploy/check-coverage.sh && gofmt/vet` etc.) and confirm it's green. Then continue with your remaining kernel work per the original dispatch (report + report mailbox path unchanged).
