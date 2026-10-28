# Ops coverage shortfall — run on merged master with Go available

The Go toolchain is now available and `bash deploy/check-coverage.sh` ran successfully on your branch. Result:

- `cmd/admin`: 0.0% coverage
- `internal/admin`: 51.7% coverage — below the 90% gate (exit 1)

This is a real coverage gap, not a missing-package failure. Raise `internal/admin` (and `cmd/admin` where the script measures it) above the 90% threshold within ops-owned files. Priorities:

1. Cover the store layer not currently exercised: token hashing/expiry paths, user approve/suspend edge cases (unknown user, idempotent transitions), history insert/read paths, and error branches from the database layer.
2. Cover server routes beyond handlers already tested: form/method handling, malformed bearer tokens, bad JSON bodies, and 404/405 behavior.
3. Cover `cmd/admin` configuration/startup validation if the script's `cmd/admin` grouping is measured — even a small test puts real numbers there instead of 0.0%.
4. Run `bash deploy/check-coverage.sh` locally (sibling packages do not exist on your branch; the script will skip-or-fail on them, which is expected at this stage) and report the per-group numbers it prints.

Do not weaken the gate or exclude code from coverage. Stay in ops-owned files, commit on the ops branch, do not merge, and report exact numbers and any code paths that cannot reasonably be covered.
