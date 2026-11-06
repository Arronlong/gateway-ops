package monitor

import (
	"context"
	"errors"
	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"strings"
	"testing"
	"time"
)

func TestUsageCountSurvivesCancellationOfOriginalCaller(t *testing.T) {
	service := &Service{}
	ch := &storage.Channel{ID: 1}
	account := storage.ChannelAccount{ID: 1}
	since, until := time.Now().Add(-time.Hour), time.Now()
	started, finish := make(chan context.Context, 1), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := &usageProgressRecorder{}
	ctx = progress.WithObserver(ctx, recorder)
	first := make(chan usageRemoteCount, 1)
	go func() {
		first <- service.cachedUsageCountWith(ctx, ch, account, since, until, func(readCtx context.Context) (int64, error) {
			started <- readCtx
			progress.Start(readCtx, progress.StageUsage, "shared count")
			select {
			case <-finish:
				return 42, nil
			case <-readCtx.Done():
				return 0, readCtx.Err()
			}
		})
	}()
	sharedCtx := <-started
	cancel()
	if sharedCtx.Err() != nil {
		t.Error("original cancellation aborted shared count")
	}
	if result := <-first; !errors.Is(result.err, context.Canceled) {
		t.Fatalf("original caller did not cancel: %#v", result)
	}
	close(finish)
	result := service.cachedUsageCountWith(context.Background(), ch, account, since, until, func(context.Context) (int64, error) {
		t.Error("shared count was repeated")
		return 0, errors.New("unexpected count")
	})
	if result.err != nil || result.count != 42 {
		t.Fatalf("manual caller inherited cancelled count: %#v", result)
	}
	if len(recorder.events) != 0 {
		t.Fatal("shared count retained an HTTP progress observer")
	}
}

type usageProgressRecorder struct{ events []progress.Event }

func (recorder *usageProgressRecorder) Emit(event progress.Event) {
	recorder.events = append(recorder.events, event)
}

func TestUsageTotalMatchSkipsMonthlyAuditForManualAndReadOnly(t *testing.T) {
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, mode := range []string{"manual", "readonly", "automatic"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newUsageHistoryFixture(t, []time.Time{since.Add(time.Hour), until.Add(-time.Hour)}, -1)
			recorder := &usageProgressRecorder{}
			ctx := progress.WithObserver(context.Background(), recorder)
			if mode == "manual" {
				if _, err := fixture.service.RefetchUsageHistoryWithReport(ctx, &fixture.ch, since, until); err != nil {
					t.Fatal(err)
				}
			} else if mode == "readonly" {
				check, err := fixture.service.CheckUsageIntegrity(ctx, &fixture.ch, since, until)
				if err != nil || check.Status != "matched" {
					t.Fatalf("check=%#v error=%v", check, err)
				}
			} else {
				if err := fixture.service.RefreshUsage(ctx, &fixture.ch); err != nil {
					t.Fatal(err)
				}
			}
			wantCounts := 2
			if mode != "readonly" {
				wantCounts = 0
			}
			if fixture.countRequests != wantCounts {
				t.Fatalf("healthy totals were expanded into %d count requests", fixture.countRequests)
			}
			if mode == "readonly" && len(fixture.reads) > 0 {
				t.Fatal("healthy totals downloaded history")
			}
			if mode == "manual" && len(fixture.reads) != 22 {
				t.Fatalf("expected 22 monthly first-page probes, got %d", len(fixture.reads))
			}
			if mode != "automatic" {
				found := false
				for _, event := range recorder.events {
					if strings.Contains(event.Message, "\u603b\u6570") {
						found = true
					}
				}
				if !found {
					t.Fatal("total check had no progress message")
				}
			}
		})
	}
}

func TestUsageMismatchReportsTotalThenCalendarLevelsBeforeCounting(t *testing.T) {
	day := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.AddDate(0, 1, 0)}, 0)
	recorder := &usageProgressRecorder{}
	ctx := progress.WithObserver(context.Background(), recorder)
	check, err := fixture.service.CheckUsageIntegrity(ctx, &fixture.ch, day.AddDate(0, -1, 0), day.AddDate(0, 2, 0))
	if err != nil || check.Status != "mismatch" {
		t.Fatalf("check=%#v error=%v", check, err)
	}
	for _, expected := range []string{"\u603b\u6570", "\u6708", "\u5468", "\u5929"} {
		found := false
		for _, event := range recorder.events {
			if strings.Contains(event.Message, expected) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing phase %q", expected)
		}
	}
}
