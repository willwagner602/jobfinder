package jobs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEngineRetryAndPartialFailures(t *testing.T) {
	t.Run("temporary status retries then succeeds", func(t *testing.T) {
		calls := 0
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls < 2 {
				http.Error(w, "temporary", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, `{"jobs":[]}`)
		}))
		defer s.Close()
		if _, err := (GreenhouseProvider{}).Fetch(context.Background(), s.Client(), Board{URL: s.URL}, time.Time{}); err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatalf("requests=%d, want retry", calls)
		}
	})
	t.Run("unsupported and failed boards reported", func(t *testing.T) {
		conn := openCurrentTestDB(t)
		insertBoard(t, conn, "https://unused.example", "Unknown", "not-real")
		insertBoard(t, conn, "http://127.0.0.1:1", "Broken", "greenhouse")
		result, err := Run(context.Background(), conn, &http.Client{Timeout: 100 * time.Millisecond}, time.Time{})
		if err == nil || result.Seen != 0 || len(result.Errors) != 2 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		if !strings.Contains(strings.Join(result.Errors, " "), "unsupported ats_type") {
			t.Fatalf("errors=%v", result.Errors)
		}
	})
	t.Run("invalid board fetch payload", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "bad", http.StatusBadRequest) }))
		defer s.Close()
		_, err := (GreenhouseProvider{}).Fetch(context.Background(), s.Client(), Board{URL: s.URL}, time.Time{})
		if err == nil || !strings.Contains(err.Error(), "status 400") {
			t.Fatalf("error=%v", err)
		}
	})
}
