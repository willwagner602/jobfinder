package jobs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemainingHelperBranches(t *testing.T) {
	if !isHackerNews(" HN ") || isHackerNews("other") {
		t.Fatal("Hacker News normalization mismatch")
	}
	if boardEndpoint("https://x.test/api/", "/api") != "https://x.test/api" || boardEndpoint("https://x.test", "/api") != "https://x.test/api" {
		t.Fatal("board endpoint mismatch")
	}
	if parseTime(" ").Valid || parseTime("not a date").Valid {
		t.Fatal("invalid dates should be null")
	}
	if !parseTime("2025-02-03").Valid {
		t.Fatal("date-only timestamp should parse")
	}
}

func TestIngestBoardFailureAndEmptyPayloads(t *testing.T) {
	conn := openCurrentTestDB(t)
	res, err := IngestBoard(context.Background(), conn, nil, Board{}, time.Time{}, fetchErrorProvider{})
	if err == nil || res.Seen != 0 {
		t.Fatalf("fetch failure result=%+v err=%v", res, err)
	}
	res, err = IngestBoard(context.Background(), conn, nil, Board{}, time.Time{}, staticProvider{})
	if err != nil || res.Seen != 0 {
		t.Fatalf("empty result=%+v err=%v", res, err)
	}
}

type fetchErrorProvider struct{}

func (fetchErrorProvider) Fetch(context.Context, *http.Client, Board, time.Time) ([]Listing, error) {
	return nil, errors.New("fetch failed")
}

func TestIngestBoardReportsUpsertFailure(t *testing.T) {
	conn := openCurrentTestDB(t)
	provider := staticProvider{listings: []Listing{{CanonicalURL: "https://example.test/jobs/1", Title: "Engineer"}}}
	result, err := IngestBoard(context.Background(), conn, nil, Board{ID: 999}, time.Time{}, provider)
	if err != nil || result.Seen != 1 || len(result.Errors) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestCommentCacheNilAndDurableLookupPaths(t *testing.T) {
	var nilCache *CommentCache
	if _, positive, found, err := nilCache.Lookup(context.Background(), "x"); err != nil || positive || found {
		t.Fatalf("nil lookup: %v %v %v", positive, found, err)
	}
	if err := nilCache.StorePositive(context.Background(), "x", Listing{}); err != nil {
		t.Fatal(err)
	}
	conn := openCurrentTestDB(t)
	cache, err := NewDurableCommentCache(context.Background(), conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, positive, found, err := cache.Lookup(context.Background(), "missing"); err != nil || positive || found {
		t.Fatalf("missing lookup: %v %v %v", positive, found, err)
	}
	if err := cache.StoreNegative(context.Background(), "negative"); err != nil {
		t.Fatal(err)
	}
	cache.positive = map[string]Listing{}
	cache.negative = map[string]bool{}
	if _, positive, found, err := cache.Lookup(context.Background(), "negative"); err != nil || positive || !found {
		t.Fatalf("negative lookup: %v %v %v", positive, found, err)
	}
	cache.positive = map[string]Listing{}
	cache.negative = map[string]bool{}
	listing := Listing{CanonicalURL: "https://example.test", Title: "Hiring", Metadata: map[string]string{"x": "y"}}
	if err := cache.StorePositive(context.Background(), "positive", listing); err != nil {
		t.Fatal(err)
	}
	cache.positive = map[string]Listing{}
	if got, positive, found, err := cache.Lookup(context.Background(), "positive"); err != nil || !positive || !found || got.Title != listing.Title {
		t.Fatalf("positive lookup=%+v %v %v %v", got, positive, found, err)
	}
}

func TestActiveBoardsAndProviderAdapterErrors(t *testing.T) {
	conn := openCurrentTestDB(t)
	if _, err := ActiveBoards(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad", http.StatusBadRequest) }))
	defer server.Close()
	if _, err := getText(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("getText should reject non-2xx")
	}
	var dst map[string]any
	if err := getJSON(context.Background(), server.Client(), server.URL, &dst); err == nil {
		t.Fatal("getJSON should reject non-2xx")
	}
}

func TestDatabaseClosedErrorPaths(t *testing.T) {
	conn := openCurrentTestDB(t)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDurableCommentCache(context.Background(), conn); err == nil {
		t.Fatal("durable cache should fail on closed connection")
	}
	if _, err := ActiveBoards(context.Background(), conn); err == nil {
		t.Fatal("ActiveBoards should fail on closed connection")
	}
}
