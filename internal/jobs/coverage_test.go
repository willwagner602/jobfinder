package jobs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProviderForMappingsAndUnknown(t *testing.T) {
	for _, name := range []string{" greenhouse ", "LEVER", "ashby", "workday", "jobvite", "climatebase", "hackernews", "hn"} {
		if provider, ok := ProviderFor(name); !ok || provider == nil {
			t.Errorf("ProviderFor(%q) = (%v, %v), want provider", name, provider, ok)
		}
	}
	if provider, ok := ProviderFor("unknown"); ok || provider != nil {
		t.Fatalf("unknown provider = (%v, %v), want nil, false", provider, ok)
	}
}

func TestRetryOutcomes(t *testing.T) {
	t.Run("zero attempts still invokes once", func(t *testing.T) {
		calls := 0
		want := errors.New("nope")
		if got := Retry(context.Background(), 0, 0, func() error { calls++; return want }); !errors.Is(got, want) || calls != 1 {
			t.Fatalf("Retry result=%v calls=%d", got, calls)
		}
	})
	t.Run("eventual success", func(t *testing.T) {
		calls := 0
		err := Retry(context.Background(), 3, 0, func() error {
			calls++
			if calls < 2 {
				return errors.New("again")
			}
			return nil
		})
		if err != nil || calls != 2 {
			t.Fatalf("Retry result=%v calls=%d", err, calls)
		}
	})
	t.Run("cancelled delay", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := Retry(ctx, 2, time.Hour, func() error { return errors.New("again") })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Retry error=%v, want canceled", err)
		}
	})
}

func TestHTTPAdaptersRejectBadResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"not found", http.StatusNotFound, `{}`},
		{"malformed json", http.StatusOK, `{`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			_, err := (GreenhouseProvider{}).Fetch(context.Background(), s.Client(), Board{URL: s.URL}, time.Time{})
			if err == nil {
				t.Fatal("Fetch succeeded, want response error")
			}
		})
	}
}
