# Admin UI fix instructions — round 1 findings

Fix both findings from `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-admin-ui--findings-r1.md`, staying strictly within `internal/admin/**`.

## Finding 1 — Unauthenticated GET routes on admin user-action paths

The four new `GET /admin/users/{approve,suspend,tombstone,untombstone}` registrations hit `methodNotAllowed` **without** `s.requireAdmin` wrapping, so an unauthenticated caller gets a 405 (not a 401) — leaking existence of the route and violating the acceptance criterion "every new route must still require admin token auth (401 without a valid bearer)".

Smallest correct fix (see the reviewer's suggestion and apply the version that keeps the auth invariant airtight):

- Wrap each of those four GET registrations in `s.requireAdmin`, i.e. route to a `requireAdmin`-wrapped handler that then returns 405 for GET. The important property: an **unauthenticated** request must get **401**, regardless of method and regardless of path; an **authenticated** caller using the wrong method gets 405.
- Verify the mux pattern doesn't already make some of these unreachable in a way that would change behavior (e.g., if the mux treats `GET /admin/users/approve` as overlapping `GET /admin/users/{user_id}`); if so, adjust the pattern so the method-blocker is genuinely reachable and doesn't shadow the per-user detail page.
- Add tests that hit **every** new admin path with **no** bearer token and assert 401 (including the GET method-blocker paths), plus a test that a valid bearer + wrong method → 405.

## Finding 2 — Admin group coverage gate dropped to 86.2% (needs >90%)

You must raise admin coverage back above the 90% gate through **tests only** — do not lower the threshold, exclude files, or weaken `deploy/check-coverage.sh`.

Concretely:

1. Run `go test ./cmd/admin ./internal/admin -coverpkg=./cmd/admin,./internal/admin -coverprofile=/tmp/admin.cov && go tool cover -func=/tmp/admin.cov | sort -k3 -n | head -25` to get the exact uncovered functions, ranked.
2. Prioritize the new routes you just added: per-user data page (`GET /admin/users/{id}` — happy path, unknown-id 404, non-numeric id 400), tombstone/untombstone handlers (success, unknown id, bad form data), signup toggle route (success + validation), and the new 401/405 boundary tests from Finding 1.
3. Also cover remaining store/server paths that the reviewer's profile lists if still needed to cross 90%.
4. Re-run `bash deploy/check-coverage.sh` locally and confirm admin group exceeds 90%.

## Commit + report

- Same file scope as before: `internal/admin/**` only.
- Run the usual chain (`bash deploy/check-coverage.sh`, `go test ./...`, `gofmt -l`, `go vet`) and confirm green.
- Commit on the `admin-ui` branch; then append an addendum to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-admin-ui/to-orchestrator--report-final.md` noting the fix commit hash and the new coverage number.
