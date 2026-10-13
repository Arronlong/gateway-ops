package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            int
	}{
		{"1.0.0", "v1.0.0", 0}, {"1.2.0", "V1.10.0", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1}, {"1.0.0-dev", "1.0.0", -1},
		{"2.0.0", "1.9.9", 1}, {"1.0.0+build.1", "1.0.0+build.2", 0},
	} {
		t.Run(tc.current+"/"+tc.latest, func(t *testing.T) {
			got, err := Compare(tc.current, tc.latest)
			if err != nil || got != tc.want {
				t.Fatalf("Compare() = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
	for _, invalid := range []string{"latest", "v1.0", "1.02.0", "1.0.0-01", "vV1.0.0"} {
		if _, err := Compare(invalid, "1.0.0"); err == nil {
			t.Fatalf("accepted invalid version %q", invalid)
		}
	}
}

func TestCheckerUsesGitHubAndCachesConcurrentChecks(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Error("missing GitHub API headers")
		}
		json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.10.0", "html_url": "https://github.com/Arronlong/gateway-ops/releases/tag/v1.10.0", "published_at": "2026-10-13T00:00:00Z"})
	}))
	defer server.Close()
	checker := NewChecker(Info{Version: "1.2.0"}, server.Client(), server.URL)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := checker.Check(context.Background())
			if err != nil || got.Status != "update_available" || got.UpdateAvailable == nil || !*got.UpdateAvailable || got.LatestVersion != "1.10.0" {
				t.Errorf("Check() = %+v, %v", got, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("GitHub calls = %d; want 1", calls.Load())
	}
}

func TestCheckerDistinguishesLatestUnavailableAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       int
		body, want string
		wantErr    bool
	}{
		{"same", 200, `{"tag_name":"v1.0.0"}`, "up_to_date", false},
		{"older", 200, `{"tag_name":"v0.9.0"}`, "current_newer", false},
		{"no release", 404, `{}`, "no_release", false},
		{"rate limited", 403, `{"message":"rate limit exceeded"}`, "check_failed", true},
		{"network gateway", 502, `error`, "check_failed", true},
		{"invalid JSON", 200, `invalid`, "check_failed", true},
		{"invalid tag", 200, `{"tag_name":"nightly"}`, "check_failed", true},
		{"prerelease", 200, `{"tag_name":"v2.0.0-rc.1","prerelease":true}`, "check_failed", true},
		{"draft", 200, `{"tag_name":"v2.0.0","draft":true}`, "check_failed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			checker := NewChecker(Info{Version: "1.0.0"}, server.Client(), server.URL)
			for i := 0; i < 2; i++ {
				got, err := checker.Check(context.Background())
				if (err != nil) != tc.wantErr || got.Status != tc.want {
					t.Fatalf("Check() = %+v, %v", got, err)
				}
				if (tc.wantErr || tc.want == "no_release") && got.UpdateAvailable != nil {
					t.Fatal("unavailable check must not claim up to date")
				}
			}
			if calls != 1 {
				t.Fatalf("calls = %d; want cached result", calls)
			}
		})
	}
}

func TestCheckerRespectsCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	checker := NewChecker(Info{Version: "1.0.0"}, &http.Client{Timeout: time.Second}, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := checker.Check(ctx)
	if err == nil || got.UpdateAvailable != nil {
		t.Fatalf("Check() = %+v, %v", got, err)
	}
}

func TestCurrentInfoDefaultsToLatestStableVersion(t *testing.T) {
	got := Current()
	if got.Name != "GatewayOps" || got.Version != "1.0.1" || got.Channel != "stable" || got.Repository != "Arronlong/gateway-ops" {
		t.Fatalf("Current() = %+v", got)
	}
}

func TestCheckerRefreshesAfterCachedFailureExpires(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"tag_name":"v1.1.0"}`))
	}))
	defer server.Close()
	checker := NewChecker(Info{Version: "1.0.0"}, server.Client(), server.URL)
	if _, err := checker.Check(context.Background()); err == nil {
		t.Fatal("expected rate limit error")
	}
	checker.expires = time.Now().Add(-time.Second)
	got, err := checker.Check(context.Background())
	if err != nil || got.Status != "update_available" || calls != 2 {
		t.Fatalf("refresh = %+v, %v; calls %d", got, err, calls)
	}
}
