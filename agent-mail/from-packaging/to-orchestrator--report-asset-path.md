# Follow-up report: SPA asset path

## Change

Updated existing PR #4 on branch `packaging` to set the web runtime image environment variable:

```Dockerfile
ENV SPA_ASSET_DIR=/app/static
```

This matches the web-hosting task contract, where `cmd/web/main.go` reads `SPA_ASSET_DIR` and otherwise defaults to `frontend/dist` for local development.

## Exact production path verified

- Built Vue assets are copied into the production web image at `/app/static`.
- The runtime image sets `SPA_ASSET_DIR=/app/static`.
- Verified inside the built image that:
  - `$SPA_ASSET_DIR` equals `/app/static`
  - `/app/static/index.html` exists
  - `/app/static/assets` exists

## Files changed

- `deploy/web.Containerfile`
- `deploy/README.md`

## Verification commands and results

1. `docker build -t jobs-web:packaging-asset-path -f deploy/web.Containerfile . && docker image inspect jobs-web:packaging-asset-path --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^SPA_ASSET_DIR=/app/static$'`
   - Exit code: 0
   - Output confirmed `SPA_ASSET_DIR=/app/static` in the image config.

2. `docker run --rm --entrypoint sh jobs-web:packaging-asset-path -c 'test "$SPA_ASSET_DIR" = /app/static && test -f /app/static/index.html && test -d /app/static/assets && printf "%s\n" "$SPA_ASSET_DIR" && ls /app/static | sed -n "1,5p"'`
   - Exit code: 0
   - Output:
     - `/app/static`
     - `assets`
     - `index.html`

3. `git diff --check`
   - Exit code: 0

## Commit and push

- Commit: `9faedb5 packaging: set runtime SPA asset path`
- Pushed to `origin/packaging`, updating existing PR #4: https://github.com/willwagner602/jobs.willwagner.me/pull/4

No merge performed.
