package relay

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestReportCanceledWaiterDoesNotCancelSharedLoad(t *testing.T) {
	service := NewService(nil, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	load := func(ctx context.Context) (any, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return "complete", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := service.cachedReport(ctx, "report", time.Minute, load); first <- err }()
	<-started
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter = %v", err)
	}
	close(release)
	result, err := service.cachedReport(context.Background(), "report", time.Minute, load)
	if err != nil || result != "complete" || calls.Load() != 1 {
		t.Fatalf("result=%v err=%v calls=%d", result, err, calls.Load())
	}
}

func TestReportCachesUpstreamFailureBriefly(t *testing.T) {
	service := NewService(nil, nil)
	var calls int
	load := func(context.Context) (any, error) { calls++; return nil, errors.New("upstream unavailable") }
	for index := 0; index < 2; index++ {
		_, err := service.cachedReport(context.Background(), "failed", time.Minute, load)
		if err == nil {
			t.Fatal("failure was hidden")
		}
	}
	if calls != 1 {
		t.Fatalf("failure retried %d times", calls)
	}
}
