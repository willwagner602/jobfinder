#!/usr/bin/env bash
set -euo pipefail

threshold="${COVERAGE_THRESHOLD:-85}"

declare -A service_packages=(
  [admin]="./cmd/admin ./internal/admin"
  [web]="./cmd/web ./internal/web"
  [board-search]="./cmd/board-search ./internal/boards"
  [job-description-search]="./cmd/job-description-search ./internal/jobs"
  [job-match-search]="./cmd/job-match-search ./internal/matching"
)

for service in admin web board-search job-description-search job-match-search; do
  packages=${service_packages[$service]}
  echo "checking ${service} coverage for: ${packages}"
  go list ${packages} >/dev/null
  coverpkg=$(printf '%s' "${packages}" | tr ' ' ',')
  go test ${packages} -coverpkg="${coverpkg}" -coverprofile="coverage-${service}.out"
  go tool cover -func="coverage-${service}.out" | tee "coverage-${service}.txt"
  awk -v service="${service}" -v threshold="${threshold}" '/^total:/ { sub(/%/, "", $3); if ($3+0 <= threshold) { printf "%s coverage %.1f%% is not greater than %.1f%%\n", service, $3, threshold; exit 1 } found=1 } END { if (!found) { printf "%s coverage total not found\n", service; exit 1 } }' "coverage-${service}.txt"
done
