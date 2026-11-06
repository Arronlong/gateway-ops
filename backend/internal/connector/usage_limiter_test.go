package connector

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUsageLimiterSharesSiteBudgetAndHonorsCancellation(t *testing.T) {
	limiter := NewUsageLimiter(25*time.Millisecond, time.Minute)
	ctx := WithUsageLimiter(context.Background(), limiter)
	if err := WaitUsageRequest(ctx, "https://example.com/channel-a"); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := WaitUsageRequest(ctx, "https://example.com/channel-b"); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 20*time.Millisecond {
		t.Fatal("requests to the same origin were not paced")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := WaitUsageRequest(canceled, "https://example.com"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	ObserveUsageResponse(ctx, "https://example.com", 429)
	if err := WaitUsageRequest(ctx, "https://example.com/other-account"); err == nil {
		t.Fatal("rate limited origin was immediately retried")
	}
	if err := WaitUsageRequest(ctx, "https://other.example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestUsageLimiterBlocksForbiddenResponsesAndRecoversAfterCooldown(t *testing.T) {
	ctx := WithUsageLimiter(context.Background(), NewUsageLimiter(0, time.Millisecond))
	ObserveUsageResponse(ctx, "https://example.com", 403)
	if err := WaitUsageRequest(ctx, "https://example.com"); err == nil {
		t.Fatal("forbidden origin was not paused")
	}
	time.Sleep(3 * time.Millisecond)
	if err := WaitUsageRequest(ctx, "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if err := WaitUsageRequest(context.Background(), "https://example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestUsageLimiterSerializesConcurrentRequestsToOneOrigin(t *testing.T) {
	ctx := WithUsageLimiter(context.Background(), NewUsageLimiter(0, time.Minute))
	release, err := BeginUsageRequest(ctx, "https://example.com/a")
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	go func() {
		finish, err := BeginUsageRequest(ctx, "https://example.com/b")
		if err == nil {
			close(acquired)
			finish()
		}
	}()
	select {
	case <-acquired:
		t.Fatal("requests overlapped")
	case <-time.After(10 * time.Millisecond):
	}
	release()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("request did not resume")
	}
}
