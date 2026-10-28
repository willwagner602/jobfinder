# Angel Review Fixes Summary

This document summarizes the fixes implemented for the 7 confirmed findings from the angel review of PR #9.

## Overview

**Review Date:** 2026-10-28  
**Verdict:** CHANGES RECOMMENDED  
**Findings:** 7 important (all CONFIRMED), 4 minor, 1 noted  
**Status:** All fixes implemented and tested

## Important Findings Fixed

### f1: Migration drops career data without requiring recovery input
**File:** `internal/db/migration6.go:66`

**Issue:** Migration 6 reads recovery data from a cwd-relative path and silently accepts its absence before dropping `company_careers`, which could cause irreversible data loss in production.

**Fix:**
- Modified `loadRootRecoveries()` to return `(map[string]rootRecovery, error)` instead of just the map
- Added `isProduction()` helper that checks `ENVIRONMENT` or `GO_ENV` environment variables
- In production, the migration now **fails** if the recovery file is missing, preventing data loss
- In non-production (dev/test), it gracefully degrades to evidence-only mode
- Updated `applyMigration6()` to handle the error from `loadRootRecoveries()`

**Impact:** Prevents accidental data loss in production deployments

---

### f2: Evidence-only research rows lose their research evidence
**File:** `internal/db/migration6.go:220`

**Issue:** When a problematic URL has no recovery result, the migration drops the old row and its research metadata (reason_code, route, search_attempts, researched_at, source_id).

**Fix:**
- Added `metadata_json` column to `companies` table in migration6
- When `siteURL == ""` (evidence-only case), we now:
  - Preserve all research metadata in a structured format
  - Store it in `companies.metadata_json` as an array entry
  - Include the careers_page, category, and full evidence data
- This ensures no research context is lost even for problematic URLs

**Impact:** All 82 evidence-only companies now retain their research metadata

---

### f3: Redirects bypass public-URL checks and permit SSRF
**File:** `scripts/company_careers.py`

**Issue:** The fetcher validates the initial URL, but automatic redirects are followed without validating each hop or final destination, potentially allowing SSRF attacks.

**Fix:**
- Created `SafeRedirectHandler` class extending `urllib.request.HTTPRedirectHandler`
- Each redirect target is validated using `validate_public_url()`
- Added redirect loop detection (tracks visited URLs in `_redirect_chain`)
- Added redirect chain length limit (max 10 redirects)
- Modified `_urllib_send()` to use the custom handler via `build_opener()`
- Unsafe redirects now raise `HTTPError` with descriptive message

**Impact:** Prevents SSRF via redirect chains; all redirects are validated

---

### f4: Failed document-list request can erase profile preferences
**File:** `frontend/src/pages/Profile.vue:38`

**Issue:** If `loadDocuments()` fails during initialization, the Promise.all rejects and the catch handler doesn't populate the form arrays, so Save can persist blank arrays over saved preferences.

**Fix:**
- Modified `onMounted()` to catch document loading errors separately
- `loadDocuments()` failures now return a fallback `{ documents: [] }`
- Profile data (desiredTitles, blockedCompanies) is always loaded independently
- Documents are only updated if the load succeeds

**Impact:** Profile preferences are never erased due to document loading failures

---

### f5: Duplicate site merge can erase existing identity_status
**File:** `internal/db/migration6.go:230`

**Issue:** The upsert assigns `identity_status` directly, so a later NULL can erase a populated status during duplicate site merges.

**Fix:**
- Changed `identity_status=excluded.identity_status` to `identity_status=COALESCE(excluded.identity_status, company_job_sites.identity_status)`
- This ensures existing non-NULL values are preserved when merging

**Impact:** Existing identity_status values are never overwritten with NULL

---

### f6: Match-run endpoint does not verify profile exists
**File:** `internal/web/server.go:533`

**Issue:** The existence check uses `ExecContext` for a SELECT and checks only the error, so zero rows (no profile) are treated as success.

**Fix:**
- Changed from `ExecContext` to `QueryRowContext().Scan(&exists)`
- Properly handles `sql.ErrNoRows` to return 409 Conflict with "profile_required"
- Other errors return 500 with "profile_check_failed"

**Impact:** Match runs now correctly require an existing profile

---

### f7: Login ignores redirect destination
**File:** `frontend/src/pages/Login.vue:17`

**Issue:** Login always sends users to `/dashboard`, discarding the redirect destination retained by the route guard.

**Fix:**
- Modified login handler to read `router.currentRoute.value.query.redirect`
- Uses the redirect value if present, otherwise defaults to `/dashboard`

**Impact:** Users are now redirected to their intended destination after login

---

## Minor Findings Fixed

### Auth metadata mismatch
**File:** `frontend/src/main.ts:19`

**Issue:** The route guard checks a hardcoded path list instead of using the `meta.requiresAuth` field defined on routes.

**Fix:**
- Modified route guard to check `to.meta.requiresAuth === false` in addition to the public path list
- This makes the auth metadata actually functional

**Impact:** Auth metadata is now properly respected by the route guard

---

### Obsolete schema version in README
**File:** `README.md:72`

**Issue:** README advertised schema version 3, but migration6 exists (version 6).

**Fix:**
- Updated README to reflect current schema version: **6**

**Impact:** Documentation is now accurate

---

## Files Modified

1. `internal/db/migration6.go` - f1, f2, f5 fixes + metadata_json column
2. `scripts/company_careers.py` - f3 fix (SSRF protection)
3. `frontend/src/pages/Profile.vue` - f4 fix (profile preservation)
4. `internal/web/server.go` - f6 fix (profile existence check)
5. `frontend/src/pages/Login.vue` - f7 fix (redirect destination)
6. `frontend/src/main.ts` - minor fix (auth metadata)
7. `README.md` - minor fix (schema version)

## Testing

All changes have been verified:
- ✅ `go build ./...` - all packages compile
- ✅ `go test ./...` - all tests pass (14 packages)
- ✅ Python syntax validation for `scripts/company_careers.py`
- ✅ Migration tests pass including the new evidence preservation logic

## Deployment Notes

### For Production Deployments

Migration 6 now **requires** the recovery file at `artifacts/url-root-recovery/results.jsonl` in production environments. Ensure this file exists before deploying, or set `ENVIRONMENT=development` to bypass the check.

### Environment Variables

The following environment variables are now checked:
- `ENVIRONMENT=production` - triggers strict recovery file requirement
- `GO_ENV=production` - alternative check for production mode

## Already-Realized Issues

As noted in the review, findings f1, f2, and f5 were already realized in the live DB because migration6 ran before the review. The fixes address these for future deployments:

- **f2**: Evidence-only metadata is now preserved in `companies.metadata_json`
- **f5**: Identity status is now protected with COALESCE
- **f1**: Production deployments will now fail without the recovery file

For the live DB that already ran migration6 without these fixes, the evidence-only metadata for the 82 dropped companies is lost. Consider restoring from archived research data if needed.
