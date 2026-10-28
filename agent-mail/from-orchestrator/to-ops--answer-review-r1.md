# Ops/admin review response — round 1

Fix both findings in `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-ops--findings-r1.md`.

1. Connect issued/revoked tokens to authorization. Protected routes should accept active DB-backed admin tokens through `Store.ValidateToken`, and revoked tokens must immediately fail. Retain `JOBS_ADMIN_TOKEN` only as a clearly separated bootstrap credential if needed. Add an end-to-end server test: issue token, authenticate with it, revoke it, then verify 401. Ensure bootstrap path is not accidentally revocable as an issued token.
2. Enforce >90% coverage per service rather than only `internal/admin`. Add CI checks for each shipped service boundary (admin, web, board-search, job-description-search, job-match-search), preferably package-group coverage with `-coverpkg` and exact deterministic thresholds. Update local docs with the same commands. Since other service packages are arriving on parallel branches, use package paths/names that exist at integration time and ensure a missing package cannot silently skip the gate; document the mapping.

Stay in ops-owned paths. Commit on `ops`, update report with verification limitations. Do not edit other task scopes.