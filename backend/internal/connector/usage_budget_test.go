package connector

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUsageRollingBudgetWaitsAndSharesAcrossAccounts(t *testing.T) {
	limiter := NewUsageLimiterWithBudget(0, time.Minute, 3, 45*time.Millisecond)
	ctx := WithUsageLimiter(context.Background(), limiter)
	started := time.Now()
	for i := 0; i < 4; i++ {
		release, err := BeginUsageRequest(ctx, "https://example.com/account")
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if time.Since(started) < 40*time.Millisecond {
		t.Fatal("requests escaped the shared rolling budget")
	}
	startOther := time.Now()
	release, err := BeginUsageRequest(ctx, "https://other.example.com")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if time.Since(startOther) > 30*time.Millisecond {
		t.Fatal("unrelated upstream shared the same budget")
	}
}

func TestUsageRejectionRetainsHTTPStatusAndRetryAfter(t *testing.T) {
	limiter := NewUsageLimiter(0, time.Minute)
	ctx := WithUsageLimiter(context.Background(), limiter)
	ObserveUsageResponse(ctx, "https://example.com", 429, "2")
	err := WaitUsageRequest(ctx, "https://example.com/a")
	var pause *UsagePauseError
	if !errors.As(err, &pause) || pause.Status != 429 || !strings.Contains(err.Error(), "429") {
		t.Fatalf("rejection lost its cause: %v", err)
	}
	remaining := time.Until(pause.Until)
	if remaining < time.Second || remaining > 3*time.Second {
		t.Fatalf("Retry-After ignored: %s", remaining)
	}
	info := limiter.WaitInfo("https://example.com/b")
	if info.Status != 429 || !info.Until.Equal(pause.Until) {
		t.Fatalf("other account lost origin rejection: %#v", info)
	}
}

func TestUsageBudgetReducesAfterUpstreamRejectsIt(t *testing.T) {
	limiter := NewUsageLimiterWithBudget(0, time.Millisecond, 4, 40*time.Millisecond)
	ctx := WithUsageLimiter(context.Background(), limiter)
	ObserveUsageResponse(ctx, "https://example.com", 429)
	time.Sleep(3 * time.Millisecond)
	started := time.Now()
	for i := 0; i < 3; i++ {
		release, err := BeginUsageRequest(ctx, "https://example.com")
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if time.Since(started) < 35*time.Millisecond {
		t.Fatal("upstream rejection did not reduce future request budget")
	}
}
