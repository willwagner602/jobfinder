# Web review request — round 1

Task `web`; branch `web`; worktree `/home/wbw/work/jobs.willwagner.me/.worktrees/web`; commit `59d2a2f`.
Spec `/home/wbw/work/jobs.willwagner.me/mvp-plan.md` (especially §1, auth acceptance, user job pages).
Worker report `/home/wbw/work/jobs.willwagner.me/agent-mail/from-web/to-orchestrator--report-final.md`.
Owned paths: `internal/web/**`, `cmd/web/**`, `templates/**`, `static/**`.

Review the diff adversarially for security and correctness: password/session design, CSRF, email confirmation/OTP flow, auth on every protected page, onboarding data safety, user-scoped listings/detail, schema compatibility and tests. Verify issues from code, not report claims. Read-only. Go tooling is unavailable. Write findings/clean report to `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-web--findings-r1.md`; return verdict and exact path.