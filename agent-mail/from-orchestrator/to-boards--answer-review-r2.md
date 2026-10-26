# Boards review response — round 2

Fix the important finding in `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-boards--findings-r2.md`:

- The daily executable currently defaults `-providers` to `fixture`, so a normal scheduled invocation does not run Adzuna or SerpAPI and can fail without a fixture path.
- Set the deployable default provider set to `adzuna,serpapi`, using the existing credential/configuration inputs. Keep fixture mode available only when explicitly requested for tests/offline use. Missing provider credentials should produce a clear startup error (or provider-level error that is recorded in process history), not silently skip required sources.
- Update CLI/configuration tests and relevant usage/docs/comments within boards-owned paths to prove default selection and explicit fixture behavior.

Stay in boards-owned files. Commit on the boards branch, update the report with verification limitations, and do not merge without a clean independent review. Go/gofmt are unavailable in this environment; run all available checks and state limits precisely.
