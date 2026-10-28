# Web task review findings — round 1

Scope reviewed: spec `mvp-plan.md` §1 / auth acceptance / user job pages; worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/web` at `59d2a2f`; owned paths `cmd/web/**`, `internal/web/**`, `templates/**`, `static/**`.

Checked:
- Read the worker report and the changed web executable, server package, tests, templates, and static CSS.
- Compared auth, onboarding, and user-scoped job pages against the spec requirements.
- Inspected shared schema compatibility for the SQL used by the web code.
- Attempted `go test ./...`; it could not run because `go` is not installed in this environment (`/usr/bin/bash: line 61: go: command not found`).

Verdict: findings.

## Findings

### Important — `cmd/web` cannot confirm or log in accounts because the wired notifier drops verification codes

- Evidence: `mvp-plan.md:34-35` requires the web server to support authentication via email one-time code or password, and requires the email address to be confirmed before the account is live. `cmd/web/main.go:19` wires the executable with `web.LogNotifier{}`. `internal/web/server.go:49-51` defines `LogNotifier.SendCode` as `return nil` and does not log, display, persist, or deliver the code anywhere outside the private `user_history` hash.
- Failure mode: running the delivered `cmd/web` binary, a new user can submit `/signup`, but the generated signup code is never sent or made available. Because `/verify` requires that code before setting `email_confirmed_at`/`live`, neither password-based nor code-based signup can complete in the actual server. The same problem blocks `/login` with “Email me a code”.
- Why the spec requires otherwise: the spec requires usable user auth and confirmed email before account activation, not just an injectable test seam.
- Smallest correct fix: wire `cmd/web` to a real configured notifier or, for a development MVP, a notifier that at least logs the plaintext code to the process log with clear non-production labeling. If no notifier is configured, fail startup instead of silently accepting signup/login requests that can never complete.

### Important — verification/login codes are reusable until expiry, so they are not one-time codes

- Evidence: `mvp-plan.md:34-35` requires email one-time code authentication. `internal/web/server.go:359-377` verifies a submitted code by scanning recent `auth_code_issued` history rows and matching hash/purpose/expiry. On success, `internal/web/server.go:372-374` records `auth_code_used`, but later verification never checks for that event and never invalidates the issued row.
- Failure mode: after a signup or login code succeeds once, the same code remains valid for every subsequent `/verify` or `/login` attempt for that user and purpose until the 15-minute expiry. A captured login code can therefore create multiple sessions, and a signup code can be replayed after first use.
- Why the spec requires otherwise: the required email OTP flow says “one-time code”; accepting the same code repeatedly violates that security property at the auth boundary.
- Smallest correct fix: store codes in a table with an identifier and consumed timestamp, or include a unique code id in `user_history` and have `verifyCode` atomically mark the matching unexpired code consumed. Reject any code that already has a corresponding consumed marker.
