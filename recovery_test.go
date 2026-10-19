package agentusage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSnapshotRecoversExpiredClaudeWithinFiveSecondsWithoutRefetchingCodex(t *testing.T) {
	fetcher, requests, codexStarts := testFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/claude" || r.Header.Get("Authorization") != "Bearer fresh-access" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":8}}`))
	})
	fetcher.readOnlyClaudeStore = true
	current := testNow
	fetcher.now = func() time.Time { return current }
	var renewed atomic.Bool
	var reads atomic.Int32
	fetcher.run = func(_ context.Context, input []byte, name string, args ...string) ([]byte, error) {
		if input != nil || name != "/usr/bin/security" || len(args) == 0 || args[0] != "find-generic-password" {
			t.Errorf("credential mutation attempted: %s %v", name, args)
		}
		reads.Add(1)
		token, expiry := "expired-access", testNow.Add(-time.Hour)
		if renewed.Load() {
			token, expiry = "fresh-access", testNow.Add(time.Hour)
		}
		return []byte(fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d}}`, token, expiry.UnixMilli())), nil
	}
	first := fetcher.Snapshot(context.Background())
	if !strings.Contains(first.Providers[1].Error, "expired") || first.Providers[0].Error != "" {
		t.Fatalf("expected healthy Codex and expired Claude: %+v", first)
	}
	renewed.Store(true)
	current = testNow.Add(4 * time.Second)
	fetcher.Snapshot(context.Background())
	if reads.Load() != 1 {
		t.Fatal("retried before the five-second error cache expired")
	}
	current = testNow.Add(5 * time.Second)
	recovered := fetcher.Snapshot(context.Background())
	claude := recovered.Providers[1]
	if claude.Error != "" || claude.Stale || len(claude.Windows) == 0 {
		t.Fatalf("Claude did not recover after five seconds: %+v", claude)
	}
	if reads.Load() != 2 || requests.Load() != 1 || codexStarts.Load() != 1 {
		t.Fatalf("reads=%d Claude requests=%d Codex starts=%d; want 2,1,1", reads.Load(), requests.Load(), codexStarts.Load())
	}
	if !recovered.Providers[0].FetchedAt.Equal(*first.Providers[0].FetchedAt) {
		t.Fatal("healthy Codex timestamp changed during Claude-only recovery")
	}
	current = testNow.Add(10 * time.Second)
	fetcher.Snapshot(context.Background())
	if requests.Load() != 1 || codexStarts.Load() != 1 {
		t.Fatal("successful recovery did not resume normal caching")
	}
}

func TestSnapshotRetriesStaleProviderWithoutPostponingHealthyRefresh(t *testing.T) {
	var fail atomic.Bool
	fetcher, requests, codexStarts := testFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":8}}`))
	})
	current := testNow
	fetcher.now = func() time.Time { return current }
	fetcher.Snapshot(context.Background())
	fail.Store(true)
	current = testNow.Add(defaultCacheTTL)
	stale := fetcher.Snapshot(context.Background()).Providers[1]
	if !stale.Stale || len(stale.Windows) == 0 {
		t.Fatalf("expected last good data on failure: %+v", stale)
	}
	current = current.Add(5 * time.Second)
	fetcher.Snapshot(context.Background())
	if requests.Load() != 3 || codexStarts.Load() != 2 {
		t.Fatalf("failed-provider retry: Claude=%d Codex=%d", requests.Load(), codexStarts.Load())
	}
	fail.Store(false)
	current = current.Add(5 * time.Second)
	recovered := fetcher.Snapshot(context.Background()).Providers[1]
	if recovered.Error != "" || recovered.Stale {
		t.Fatalf("stale data did not recover: %+v", recovered)
	}
	current = testNow.Add(2 * defaultCacheTTL)
	fetcher.Snapshot(context.Background())
	if codexStarts.Load() != 3 || requests.Load() != 4 {
		t.Fatalf("independent success deadlines: Claude=%d Codex=%d; want 4,3", requests.Load(), codexStarts.Load())
	}
}

func TestFastRecoveryHonorsRetryAfterDeadline(t *testing.T) {
	var limited atomic.Bool
	limited.Store(true)
	fetcher, requests, codexStarts := testFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		if limited.Load() {
			w.Header().Set("Retry-After", "30")
			http.Error(w, "limited", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":8}}`))
	})
	current := testNow
	fetcher.now = func() time.Time { return current }
	fetcher.Snapshot(context.Background())
	limited.Store(false)
	for _, seconds := range []int{5, 10, 15, 20, 25, 29} {
		current = testNow.Add(time.Duration(seconds) * time.Second)
		fetcher.Snapshot(context.Background())
	}
	if requests.Load() != 1 || codexStarts.Load() != 1 {
		t.Fatal("fast polling ignored Retry-After or refetched healthy Codex")
	}
	current = testNow.Add(30 * time.Second)
	recovered := fetcher.Snapshot(context.Background()).Providers[1]
	if recovered.Error != "" || requests.Load() != 2 || codexStarts.Load() != 1 {
		t.Fatalf("did not retry at the backoff deadline: %+v", recovered)
	}
}
