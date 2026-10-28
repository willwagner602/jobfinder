# Packaging commands for UI browser checks

Please wire these commands for the UI checks owned by PR #3:

```sh
cd frontend
npm ci
npm test
npm run test:e2e
npm run build
npm run screenshots
```

Notes:

- `npm run test:e2e` runs Playwright against a built Vite preview server and route-level mocked `/api/**` responses.
- `npm run screenshots` serves `frontend/dist`, drives Chromium, and writes PNG screenshots plus `artifacts/ui-screenshots/manifest.json`.
- Browser resolution: Playwright uses its bundled browser when present, or `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`, `CHROMIUM_PATH`, or `/snap/bin/chromium`.
- No production DB, SMTP, credentials, or real verification codes are used.

Final bounded screenshot retry result after stale-process cleanup:

- Command: `cd frontend && npm run screenshots`
- Exit status: 0
- Output: `Generated 92 browser screenshots from the Vue app in /home/wbw/work/jobs.willwagner.me/.worktrees/ui-tests/artifacts/ui-screenshots`
- Verification: 92 PNG files, 92 manifest entries, 0 missing manifest files.
- Cleanup: no remaining `generate-screenshots.mjs` process after the run; the generator now uses an ephemeral local static-server port by default and closes the server in `finally`.
