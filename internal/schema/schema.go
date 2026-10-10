package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// CurrentVersion is the schema version expected by all services in this build.
const CurrentVersion = 3

type ProcessStatus string

const (
	ProcessStatusSuccess ProcessStatus = "success"
	ProcessStatusFailure ProcessStatus = "failure"
	ProcessStatusPartial ProcessStatus = "partial"
)

type BoardStatus string

const (
	BoardStatusDiscovered  BoardStatus = "discovered"
	BoardStatusActive      BoardStatus = "active"
	BoardStatusUnsupported BoardStatus = "unsupported"
)

type UserStatus string

const (
	UserStatusPending   UserStatus = "pending"
	UserStatusLive      UserStatus = "live"
	UserStatusSuspended UserStatus = "suspended"
)

type JobMatchDecision string

const (
	JobMatchDecisionMatch    JobMatchDecision = "match"
	JobMatchDecisionNoMatch  JobMatchDecision = "no_match"
	JobMatchDecisionFailure  JobMatchDecision = "failure"
	JobMatchDecisionDeferred JobMatchDecision = "deferred"
)

var (
	tagRE        = regexp.MustCompile(`<[^>]+>`)
	whitespaceRE = regexp.MustCompile(`\s+`)
)

// NormalizeBodyText strips HTML tags and collapses whitespace so markup-only
// changes do not create new listing versions.
func NormalizeBodyText(body string) string {
	withoutTags := tagRE.ReplaceAllString(body, " ")
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(withoutTags, " "))
}

// ListingContentHash returns the stable content hash for version decisions.
// The canonical listing identity is the URL stored on job_listings; title is
// intentionally only part of the version hash.
func ListingContentHash(title, salary, body string) string {
	parts := []string{strings.TrimSpace(title), strings.TrimSpace(salary), NormalizeBodyText(body)}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
