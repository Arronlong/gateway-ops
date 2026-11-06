package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestUsageAuditRetainsKnownTotalsWhenMonthlyCountsConflict(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.Add(time.Hour)}, -1)
	root := 1
	fixture.rootCount = &root
	result, err := fixture.service.CheckUsageIntegrity(context.Background(), &fixture.ch, time.Time{}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || result.Status != "mismatch" || result.UpstreamCount == nil || *result.UpstreamCount != 1 || result.LocalCount != 2 {
		t.Fatalf("confirmed total difference was erased: result=%#v err=%v", result, err)
	}
	if len(result.Accounts) != 1 || result.Accounts[0].UpstreamCount == nil || !strings.Contains(result.Accounts[0].Error, "1") || !strings.Contains(result.Accounts[0].Error, "2") {
		t.Fatalf("count conflict did not retain both numeric totals: %#v", result.Accounts)
	}
}

func TestUsageRepairWritesFirstAnomalyBeforeInspectingLaterMonths(t *testing.T) {
	day := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.AddDate(0, 2, 0)}, -1)
	if err := fixture.db.Where("channel_id = ?", fixture.ch.ID).Delete(&storage.ChannelUsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	savedBeforeLaterMonth := false
	fixture.beforeRead = func() {
		if len(fixture.reads) == 3 {
			n, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", day, day.AddDate(0, 1, 0))
			savedBeforeLaterMonth = n == 1
		}
	}
	recorder := &usageProgressRecorder{}
	ctx := progress.WithObserver(context.Background(), recorder)
	if err := fixture.service.RefetchUsageHistory(ctx, &fixture.ch, day.AddDate(0, -1, 0), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 4 || !savedBeforeLaterMonth || fixture.countRequests != 0 {
		t.Fatalf("month was not saved before checking later months: saved=%v reads=%v", savedBeforeLaterMonth, fixture.reads)
	}
	foundTotals, foundRepair := false, false
	for _, event := range recorder.events {
		if data, ok := event.Data.(map[string]any); ok && data["kind"] == "usage_integrity" {
			if data["phase"] == "locating" {
				foundTotals = true
			}
			if data["phase"] == "repairing" {
				foundRepair = true
				if !foundTotals {
					t.Fatal("repair progress preceded total snapshot")
				}
			}
		}
	}
	if !foundTotals || !foundRepair {
		t.Fatal("structured total and repair results were not streamed")
	}
}

func TestUsageRepairUsesMonthlyPagesInsteadOfConflictingGlobalSummary(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.Add(time.Hour)}, -1)
	root := 1
	fixture.rootCount = &root
	if err := fixture.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: fixture.ch.ID, ChannelAccountID: fixture.account.ID, Source: "newapi", RemoteUsageID: "extra", OccurredAt: day.Add(2 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, time.Time{}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 2 || fixture.countRequests != 0 {
		t.Fatalf("received pages were downloaded repeatedly: %v", fixture.reads)
	}
	count, err := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", time.Time{}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || count != 2 {
		t.Fatalf("valid page did not repair local history: count=%d err=%v", count, err)
	}
}

func TestUsageDenseWindowRejectsConflictingChildTotals(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	walker := usageHistoryWalker{Report: &UsageHistoryReport{}, SplitDense: true, Count: func(_ context.Context, w usageHistoryWindow) (usageWindowCounts, error) {
		if w.Until.Sub(w.Since) == 24*time.Hour {
			return usageWindowCounts{Remote: 3000}, nil
		}
		return usageWindowCounts{Remote: 1600}, nil
	}}
	_, err := walker.walk(context.Background(), usageHistoryWindow{Since: day, Until: day.Add(24 * time.Hour)}, "day", nil)
	if err == nil || !strings.Contains(err.Error(), "3000") || !strings.Contains(err.Error(), "3200") {
		t.Fatalf("dense count conflict was accepted: %v", err)
	}
}

func TestUsageRepairRejectsOrphanedDifferencesAndBypassesOldTotals(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	for _, mode := range []string{"orphan", "stale"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newUsageHistoryFixture(t, []time.Time{day.Add(time.Hour)}, -1)
			if _, err := fixture.service.CheckUsageIntegrity(context.Background(), &fixture.ch, day, day.Add(24*time.Hour)); err != nil {
				t.Fatal(err)
			}
			if mode == "orphan" {
				if err := fixture.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: fixture.ch.ID, ChannelAccountID: 999, Source: "newapi", RemoteUsageID: "orphan", OccurredAt: day.Add(2 * time.Hour)}}); err != nil {
					t.Fatal(err)
				}
				if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, day, day.Add(24*time.Hour)); err == nil {
					t.Fatal("orphaned difference was reported as repaired")
				}
			} else {
				fixture.remote = append(fixture.remote, day.Add(2*time.Hour))
				if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, day, day.Add(24*time.Hour)); err != nil {
					t.Fatal(err)
				}
				count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", day, day.Add(24*time.Hour))
				if count != 2 {
					t.Fatalf("manual repair reused old count: %d", count)
				}
			}
		})
	}
}

func TestUsageAllHistoryRejectsInflatedTotalWithoutScanningEmptyPast(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day.Add(time.Hour), day.Add(2 * time.Hour)}, -1)
	root := 3
	fixture.rootCount = &root
	fixture.rootUntil = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	result, err := fixture.service.CheckUsageIntegrity(context.Background(), &fixture.ch, time.Time{}, fixture.rootUntil)
	if err != nil || result.Phase != "interrupted" || result.UpstreamCount == nil || *result.UpstreamCount != 3 {
		t.Fatalf("inflated root accepted: %#v err=%v", result, err)
	}
	if fixture.countRequests > 8 {
		t.Fatalf("conflict scanned old empty history: %d requests", fixture.countRequests)
	}
}
