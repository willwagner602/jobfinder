# Jobs coverage follow-up — close the remaining gap to >90%

Group total is now 80.4% (up from 69.7%); merged to master as an improvement. The gate still needs >90%. Remaining gaps from the cover profile (sorted, biggest first inside each file):

1. `cmd/job-description-search/main.go:19` `main` is 0% — the cmd/admin pattern applies: refactor `main()` to call a testable `run()` returning an error, add the subprocess/`exec`-style or direct `run()` test so the cmd statements count. `startHealth` is 87.5% and `recordHistory` 100% — cover the remaining startHealth branch.
2. `internal/jobs` partial functions, biggest contributors: `NewDurableCommentCache` 66.7%, `isHackerNews` 66.7%, `boardEndpoint` 75.0%, `parseTime` 75.0%, `IngestBoard` 77.8%, `getJSON` 78.6%, `getText` 80.0%, `ActiveBoards` 81.8%, `Run` 82.1%, `HackerNewsProvider.Fetch` 83.3%, `listingFromHNComment` 83.3%, `CommentCache.Lookup` 84.6%, plus provider `Fetch` implementations at 85.7/90%.
3. Write targeted tests for those exact uncovered branches (error returns, nil/empty inputs, alternate code paths), re-measure with the group command including `-coverpkg`, and iterate until `go tool cover -func` reports `total: 91%+`.

Do not weaken the gate; stay in jobs-owned files (internal/jobs/**, cmd/job-description-search/**); commit on the jobs branch; do not merge; report exact final group total. Production refactors are allowed only where needed for testability (mirroring the cmd/admin main→run pattern).
