# Matching task final report

Task: `matching`
Latest commit: `717a38d` (`Use LLM classifier for default matching`)
Previous implementation commit: `e29963c` (`Implement matching service task`)

## Review round 1 fix

Addressed finding: executable default matcher was keyword logic instead of provider-neutral LLM classification.

Changes made:

- Added `internal/matching/llm.go` with an OpenAI-compatible HTTP chat-completions classifier behind the existing `matching.Classifier` interface.
- Added configurable LLM settings from environment:
  - `JOB_MATCH_LLM_URL` — full chat-completions endpoint URL.
  - `JOB_MATCH_LLM_MODEL` — model name sent in the request.
  - `JOB_MATCH_LLM_API_KEY` — secret sent only as `Authorization: Bearer ...`.
  - `JOB_MATCH_LLM_TIMEOUT` — optional Go duration, default `30s`.
- Added validation for required URL/model/key and absolute `http`/`https` endpoint URLs.
- Added bounded HTTP client timeout, response body limit, non-2xx provider errors, empty-content errors, malformed JSON errors, and invalid decision/rating errors.
- Ensured provider errors return from `Classify`, so the existing engine retry/backoff and failure persistence path handles them.
- Updated `cmd/job-match-search` so matcher name `jev` registers `*matching.LLMClassifier`, not keyword logic.
- Removed the executable keyword classifier.

## Original matching implementation retained

- `internal/matching` engine with provider-neutral `Classifier` interface.
- User matcher selection: per-user `user_configs.matcher_name` overrides global default `jev`.
- High-level user filters before classifier calls:
  - `blocked_companies_json` persists `no_match`.
  - `desired_titles_json` filters listings whose title does not contain a desired title.
- Match persistence into `job_matches` with raw result JSON and metadata JSON.
- Preserved `match`, `no_match`, and `failure`; exhausted classifier errors persist as `failure`, never as `no_match`.
- Classifier retry with exponential backoff and `job_history` reporting.
- Separate executable at `cmd/job-match-search` with `-db`/`JOB_DB_PATH` and `-matcher` options.

## Tests added

Existing deterministic engine tests in `internal/matching/matching_test.go` cover:

- Global default matcher vs user-level matcher override selection.
- High-level filters persisting `no_match` without calling a classifier.
- Retry/backoff followed by successful match persistence.
- Exhausted classifier failures persisted as `failure` with partial `job_history` reporting.

Review-fix tests added:

- `internal/matching/llm_test.go`
  - Uses `httptest` to verify the provider-neutral HTTP request contract: POST, bearer auth, model, messages, JSON response-format request, and prompt content.
  - Parses deterministic `match` and `no_match` provider responses.
  - Verifies HTTP provider failures and malformed classifier JSON return errors for retry/failure handling.
  - Verifies LLM configuration validation.
- `cmd/job-match-search/main_test.go`
  - Verifies default matcher `jev` is wired to `*matching.LLMClassifier`.
  - Verifies missing LLM configuration fails instead of silently falling back to keyword logic.

No test calls a live provider.

## Checks run

- `git diff --check` — passed before both attempted Go commands.
- Attempted `gofmt -w ... && go test ./...` — failed immediately because `gofmt` is unavailable: `/usr/bin/bash: line 61: gofmt: command not found`.
- Previous probe confirmed no Go toolchain on PATH: `command -v go` and `command -v gofmt` produced no paths.
- LSP diagnostics for new Go files — could not provide compile diagnostics; language server reported: `No active builds contain ... consider opening a new workspace folder containing it (go list)`.

## Verification limitations

- I could not run `gofmt`, `go test ./...`, or compile checks in this environment because the Go toolchain is not installed/available on PATH.
- The code was manually inspected and `git diff --check` passed, but formatting and full compile/test verification still need to run in an environment with Go installed.

## Files changed by latest review-fix commit

- `internal/matching/llm.go`
- `internal/matching/llm_test.go`
- `cmd/job-match-search/main.go`
- `cmd/job-match-search/main_test.go`
