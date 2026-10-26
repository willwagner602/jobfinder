# Final report — admin-interface-spec.md

## Status: complete and pushed to GitHub (master at 052e5a5)

All work streams merged to master after independent clean reviews (max-3-round loop), then a clean combined integration review verified cross-task consistency end to end.

## Shipped (per spec section)

### Admin interface (User management + Job management)

- **Schema v4** (`87e1320`): `site_config` single-row table with `signups_disabled` (0/1 CHECK, seeded default), tombstoned as a distinct 4th user status, and the partial unique email index `idx_users_email_active` (`WHERE status <> 'tombstoned'`) so tombstoned rows release their email while suspended/pending rows still block re-signup.
- **Admin UI** (`b5ce2f0`): per-user Disable (suspend) / Enable (approve) / Tombstone / Untombstone actions with warning copy distinguishing suspension vs tombstoning, bounded per-user data page reachable by clicking the user's name (users + user_configs + user_history + latest 25 job_matches), a global signup kill-switch in the sidebar, collapsible left navigation (Users / Tokens / Process history), dark-mode toggle, and rounded-card light-mode styling.
- **Ops route alignment** (`d4bcfb5`): admin signup toggle normalized to `POST /admin/config/signups` with `disabled=true|false`, redirect `/admin`, and tests/sidebar updated to match.
- **Job management** section: no delta required — the existing `/admin/process-history` page already renders per-run start, end, status, counts, and error details as the spec describes.

### Integration testing framework

- **`internal/itests/**`** (`fa96e08`): real-subprocess harness that builds and boots `cmd/web` + `cmd/admin` as OS subprocesses against isolated per-test temp SQLite DBs on ephemeral localhost ports; stdlib `net/http` with cookie jars; a stdlib fake SMTP server for real code delivery; no UI automation framework.
- **CI**: single new `Integration tests` step in `.github/workflows/ci.yml` (`go test ./internal/itests/...`), no workflow rewrite.
- **Convention doc**: `internal/itests/README.md` names future-flow requirements so reviewers can reject new UI flows without a test.
- **Coverage**: all seven specified UI flows have **passing** integration assertions on merged master (the two originally documented skips are now real tests proving web-side enforcement of both policies after the W2 merge).

### Web-side policy enforcement (W2, `9735897` — the cross-service work)

- Public signup handler **reads `site_config.signups_disabled`** and refuses the insert/code (403, generic message) when set.
- Public signup handler **refuses tombstoned-email re-signups** (403, generic message) per `admin-interface-spec.md:9`.
- Both checks run inside one pinned `BEGIN IMMEDIATE` SQLite transaction with the user upsert, auth-code insert, and history insert, so a concurrent admin tombstone cannot interleave between the check and the insert (regression tests would fail if reverted).
- Verification code is generated and persisted inside the transaction but **sent only after COMMIT**, so a post-commit SMTP failure cannot roll back durable user/code/history rows (tests prove both ordering and durability).

## Final verification on merged master (independently run by the combined reviewer)

- `go test ./...` — 0 test failures (all packages)
- `bash deploy/check-coverage.sh` — exit 0; every service group >90% (combined-group totals included 92.2%)
- `gofmt -l` over tracked files — clean
- `go vet ./...` — clean
- `go test ./internal/itests/...` (uncached, verbose) — **all seven** flow tests execute and pass, including the two previously skipped cross-service policies
- Cross-task contract check — the sidebar form, admin handler, route registration, and route tests all agree on `POST /admin/config/signups` + `disabled=true|false`; no active stale `/admin/signups` references remain in live code

## Review rounds consumed

- K: 2 rounds (round-1 finding: tombstone must release email; round-2 finding: web upsert needed index-predicate conflict target; both fixed)
- U (admin UI): 2 rounds (round-1 findings: unauthenticated GET 405-leak, coverage gap; both fixed + re-reviewed)
- I (integration framework): 1 round (clean)
- W2 (web-side enforcement): 3 rounds (round-1: check-then-insert race → atomic; round-2: send-before-commit → commit-then-send; round-3: clean)
- Combined integration: 1 round (clean)

## Spec gaps remaining

None. All spec acceptance paths have explicit evidence in this build; the original `mvp-plan.md` acceptance items already merged earlier remain unchanged (10/12 checked there; GHCR publish and live mail send were separately proven locally during deployment wiring).
