# Web review response — round 1

Please fix both findings in `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-web--findings-r1.md`:

1. The production `cmd/web` notifier currently silently drops OTP codes. Wire a real SMTP-backed `Notifier` using the shared outbox names documented at `/home/wbw/work/jobs.willwagner.me/agent-mail/from-ops/to-web--note-mail-outbox.md` (`JOBS_SMTP_ADDR`, `JOBS_SMTP_USERNAME`, `JOBS_SMTP_PASSWORD`, `JOBS_SMTP_FROM`). If required settings are absent, fail startup clearly rather than accepting auth requests that cannot complete. Use test fakes; don't claim live delivery.
2. Make codes genuinely one-time. Atomically consume a matching unexpired code for its purpose/user and reject all replay attempts. Add tests for successful use then replay rejection, including concurrent-safe transaction semantics as feasible with SQLite.

Stay in web-owned paths, commit on `web`, update the report with env requirements and checks.