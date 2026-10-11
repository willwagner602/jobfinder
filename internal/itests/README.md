# Integration tests

This package contains subprocess integration tests for user-facing and admin HTTP flows.

## What these tests cover

Each test boots the real `cmd/web` and `cmd/admin` binaries as operating-system subprocesses. The services share a per-test temporary SQLite database and listen on per-test localhost ports. Tests drive the services with the standard library `net/http`; do not use browser automation or in-process handlers here.

The current flow tests cover:

- signup, verification, login, and logout;
- login followed by jobs list and jobs detail pages with seeded listing data;
- admin user list authorization and bearer-token access;
- suspend/approve as the current disable/re-enable contract, with web login rejection while suspended;
- tombstoned-user login rejection, with the current same-email signup behavior documented as skipped until the web/schema contract rejects it;
- admin process-history counts;
- `site_config.signups_disabled`, documented as skipped until the web signup endpoint enforces it.

## Adding a new UI-flow integration test

1. Add a test named `Test<FlowName>` in `integration_test.go`. Use product behavior in the name, for example `TestAdminProcessHistoryPageRendersCounts`.
2. Call `t.Parallel()` unless the flow intentionally depends on a shared external resource. The harness is designed for isolated temporary databases and ports per test.
3. Start services with `h := newHarness(t)` and `defer h.Close()`. Use `newHarnessWithDBSetup` only when a row must exist before either service boots.
4. Use `newClient(t)` for an HTTP client with a cookie jar. Get CSRF cookies with `getCSRF`, submit forms with `postForm`, and use `getAdmin` or `postAdminForm` for bearer-authenticated admin requests.
5. Seed durable data through the real HTTP flow when the flow depends on web behavior. Use direct SQL only for setup that is not itself the behavior under test, such as seeded job listings or process-history rows.
6. Assert rendered content from the HTTP response body, not internal function return values.
7. If a spec flow depends on a contract that is not present yet, skip the test with a message naming the missing contract. Do not fake future behavior in this package.

## Running locally

From the repository root:

```sh
go test ./internal/itests/...
```

The package builds `cmd/web` and `cmd/admin` into a temporary binary directory, starts them with ephemeral localhost ports, and removes per-test databases through `testing.T.TempDir` cleanup.

## CI convention

CI must include a separate step that invokes this package directly:

```yaml
- name: Integration tests
  run: go test ./internal/itests/...
```

When adding a future UI flow, include its integration test in the same change as the UI behavior. Reviewers should reject new UI flows that do not add or update an integration test here.
