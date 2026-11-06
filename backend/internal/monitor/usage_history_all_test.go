package monitor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestUsageHistoryAllWindow(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 6, 7, 30, 0, 0, location)
	tests := []struct {
		name      string
		preset    string
		wantSince time.Time
		wantUntil time.Time
	}{
		{name: "today", preset: "today", wantSince: time.Date(2026, 11, 6, 0, 0, 0, 0, location)},
		{name: "24 hours", preset: "24h", wantSince: now.Add(-24 * time.Hour)},
		{name: "7 days", preset: "7d", wantSince: now.Add(-7 * 24 * time.Hour)},
		{name: "all", preset: "all", wantSince: time.Time{}, wantUntil: now.Add(-2 * time.Minute).UTC().Truncate(5 * time.Minute)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			since, until, err := UsageHistoryBounds(tt.preset, now, location)
			if err != nil {
				t.Fatal(err)
			}
			if !since.Equal(tt.wantSince) {
				t.Fatalf("since=%s, want %s", since, tt.wantSince)
			}
			wantUntil := tt.wantUntil
			if wantUntil.IsZero() {
				wantUntil = now
			}
			if !until.Equal(wantUntil) {
				t.Fatalf("until=%s, want %s", until, wantUntil)
			}
		})
	}
	if _, _, err := UsageHistoryBounds("30d", now, location); err == nil {
		t.Fatal("expected unsupported preset error")
	}
}

func TestRunAllUsageHistoryContinuesAfterChannelFailure(t *testing.T) {
	channels := []storage.Channel{
		{ID: 1, Name: "ok", BalanceMode: storage.BalanceModeAuto},
		{ID: 2, Name: "failed", BalanceMode: storage.BalanceModeAuto},
		{ID: 3, Name: "manual", BalanceMode: storage.BalanceModeManual},
	}
	calls := []uint{}
	result := runAllUsageHistory(context.Background(), channels, time.Unix(100, 0), time.Unix(200, 0), func(_ context.Context, channel *storage.Channel, _, _ time.Time) error {
		calls = append(calls, channel.ID)
		if channel.ID == 2 {
			return errors.New("upstream failed")
		}
		return nil
	})
	if result.Total != 3 || result.Succeeded != 1 || result.Failed != 1 || result.Skipped != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !reflect.DeepEqual(calls, []uint{1, 2}) {
		t.Fatalf("calls=%v", calls)
	}
	if len(result.Errors) != 1 || result.Errors[0].ChannelID != 2 {
		t.Fatalf("errors=%+v", result.Errors)
	}
	if len(result.SkippedChannels) != 1 || result.SkippedChannels[0].ChannelID != 3 {
		t.Fatalf("skipped channels=%+v", result.SkippedChannels)
	}
}

func TestUsageHistoryAllTodayUsesBeijingMidnightForUTCClock(t *testing.T) {
	now := time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)
	since, until, err := UsageHistoryBounds("today", now, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 5, 16, 0, 0, 0, time.UTC)
	if !since.Equal(want) || !until.Equal(now) {
		t.Fatalf("window=%s..%s, want %s..%s", since, until, want, now)
	}
}

func TestRunAllUsageHistoryStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	channels := []storage.Channel{{ID: 1}, {ID: 2}}
	calls := 0
	result := runAllUsageHistory(ctx, channels, time.Unix(100, 0), time.Unix(200, 0), func(context.Context, *storage.Channel, time.Time, time.Time) error {
		calls++
		cancel()
		return nil
	})
	if calls != 1 || result.Succeeded != 1 || result.Failed != 1 || len(result.Errors) != 1 || !errors.Is(result.Errors[0].Err, context.Canceled) {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
}

func TestRefetchAllUsageHistoryUsesSharedAllCutoff(t *testing.T) {
	since, until, err := UsageHistoryBounds("all", time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newUsageHistoryFixture(t, []time.Time{until.Add(-time.Hour)}, 0)
	fixture.service.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	second := fixture.ch
	second.ID = 0
	second.Name = "history-second"
	if err := fixture.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	account := fixture.account
	account.ID = 0
	account.ChannelID = second.ID
	if err := fixture.db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	result, err := fixture.service.RefetchAllUsageHistory(context.Background(), since, until)
	if err != nil || result.Succeeded != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	rootReads := 0
	for _, window := range fixture.reads {
		if window.Until.After(until) {
			t.Fatalf("read window exceeds the shared cutoff: %s > %s", window.Until, until)
		}
		if window.Until.Equal(until) {
			rootReads++
		}
	}
	if rootReads < 2 {
		t.Fatalf("expected both channels to read through %s, reads=%+v", until, fixture.reads)
	}
}
