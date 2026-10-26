# Remaining work to fully satisfy admin-interface-spec.md

Status: the admin-side UI (tombstone/disable/per-user page/signup toggle/sidebar+dark mode) and the integration test harness are merged to master and gate-green. What remains is the **web-side** wiring the spec requires but that does not belong to admin's file scope.

## 1. Web-side signup kill-switch enforcement (`site_config.signups_disabled`)

Currently: admin can toggle the flag via `POST /admin/signups` (wired to `Store.SetSignupDisabled`); the itests harness confirms the flag is set in the DB; but **web does not read it**, so a disabled signups setting does not actually block the public signup form.

Required work (web-owned files):
- In `internal/web/server.go`, before the signup POST handler inserts a user, read `site_config.signups_disabled` from the shared DB (single-row table seeded by kernel v4; use `Store.IsSignupDisabled`-equivalent read at the web level or a small direct SQL read matching the v4 schema).
- When set, return a 4xx (pick what fits the web service's existing style, likely 403) from `POST /signup`, and render a clear message on the signup page (spec allows either making the page inaccessible or rejecting all POSTs; rejecting POSTs is the minimum).
- Kernel contract to keep: `schema.UserStatusTombstoned` and the partial email index must not interfere — a signup attempt when signups are disabled should not accidentally bypass the tombstone/suspended checks (they are separate conditions with separate failure reasons).

## 2. Web-side tombstoned-email re-signup refusal

Currently: kernel's partial index (`idx_users_email_active WHERE status <> 'tombstoned'`) *releases* the email when a user is tombstoned, but web's signup insert has no tombstone-aware refusal, so a tombstoned user's email **can** be reused — the opposite of the spec (`admin-interface-spec.md:9`).

Spec-compliant web announcement, decided: **tombstoning permanently blocks re-signup with that email**, correct spec behavior (not the current allow). This must be enforced *in web's signup handler* (a pre-insert check that returns a clear "email not accepted" style error, NOT a schema-level change — the partial index is correct and must stay as-is so a second admin user can share the email under different semantics if ever needed).

Required work (web-owned files): in `internal/web/server.go`'s signup path, before insert, query for an existing row with that email; if found and its status is `tombstoned`, refuse the signup with a 4xx and a message that does not leak whether an account existed; do not change the SQL/index or attempt a schema workaround.

## 3. Integration tests proving items 1 and 2

Currently the itests leaf wrote two **explicitly skipped** tests for exactly these behaviors:
- `TestSignupDisabledSiteConfigBlocksSignupWhenWebEnforcesIt`
- `TestTombstonedUserLoginRejectedAndSignupPolicyDocumented` (its re-signup assertion is the skip)

Once both web-side enforcements land, unskip/extend both tests so they become real passing assertions rather than skips (columns/resource notes in `internal/itests/README.md` already describe how a future flow is added).

## 4. Remaining acceptance checklist for this spec (once 1–3 land)

- `admin-interface-spec.md` has no checkbox list of its own; the acceptance is: all controls exist and work (done, merged), modern look-and-feel (done, merged), collapsible sidebar with dynamic links (done, merged), **automated CI integration tests over the entire existing set of UI flows** (5/7 passing; items 1–3 above close the remaining 2).
- Final combined review should re-verify the whole build with `go test ./...`, `bash deploy/check-coverage.sh`, `gofmt -l`, `go vet ./...`, plus a round of interaction checks on merged master (signups-disabled toggle → web refuses POST /signup; tombstone user → re-signup refused; per-user page loads).

## Immediate next action

Open a single, narrowly-scoped web-side task ("W2") that does items 1 and 2 (only `internal/web/server.go` and its tests), then asks the itests owner to unskip/extend the two tests as a follow-up commit in the same PR/branch since both files are in play at once. Do not include admin work, don't touch deploy/CI beyond what's already merged, don't touch `internal/schema/**` or `internal/db/**`.
