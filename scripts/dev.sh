#!/usr/bin/env bash
set -euo pipefail

# Development stack launcher (docker-compose.dev.yml).
# - Fixed project name (jobfinder-dev) and fixed volume (jobfinder-dev-data),
#   so state persists no matter which directory you launch from.
# - Web on http://localhost:8090, admin on http://localhost:8091
#   (admin token defaults to dev-admin-token).
# - Never touches the production stack (project "jobfinder", ports 8080/8081,
#   volume jobfinder_jobs-data).

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

COMPOSE=(docker compose -f docker-compose.dev.yml)
command="${1:-up}"
case "$command" in
  up)
    "${COMPOSE[@]}" up --build -d
    for attempt in {1..60}; do
      if curl --fail --silent http://localhost:8090/healthz >/dev/null; then
        printf 'Dev app is ready: http://localhost:8090 (admin: http://localhost:8091/admin)\n'
        exit 0
      fi
      sleep 1
    done
    echo 'Dev web service did not become healthy. Check with: ./scripts/dev.sh logs' >&2
    exit 1
    ;;
  down)
    "${COMPOSE[@]}" down
    ;;
  reset)
    # DANGEROUS: wipes the dev database volume (prod is never touched).
    "${COMPOSE[@]}" down -v
    ;;
  status)
    "${COMPOSE[@]}" ps
    ;;
  logs)
    "${COMPOSE[@]}" logs --follow --tail=100 "${2:-web}"
    ;;
  *)
    echo 'Usage: ./scripts/dev.sh [up|down|reset|status|logs [service]]' >&2
    echo '  up      build + start dev stack (ports 8090/8091)' >&2
    echo '  down    stop dev stack, keep dev database' >&2
    echo '  reset   stop dev stack AND wipe dev database' >&2
    echo '  status  show dev stack containers' >&2
    echo '  logs    follow dev service logs' >&2
    exit 2
    ;;
esac
