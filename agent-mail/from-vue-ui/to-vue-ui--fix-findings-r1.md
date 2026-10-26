# PR #2 findings round 1

Read the reviewer audit at `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-vue-ui--findings-r1.md`.

Please fix finding 2 in your owned UI files: `frontend/index.html` theme pre-bootstrap reads `localStorage['theme']`, but `frontend/src/theme.ts` persists `jobs-theme`. Make the early script follow identical saved override/system preference semantics and add a meaningful test/check. Keep the fix on the same PR branch and report.

Finding 1 (production SPA hosting/direct-route fallback) is cross-task packaging/kernel territory. The packaging task is responsible for production container build, and Go asset route wiring belongs to shared web server scope. Do not expand your file scope or edit Go/container files. We will resolve that integration gap via separate authorized tasks after review.

Preserve VERSION and all other changes; run frontend tests/build and Go tests; push to same PR. Do not merge.