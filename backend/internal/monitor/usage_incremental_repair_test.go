package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestUsageIncrementalRepairKeepsCompletedMonthAfterLaterRejection(t *testing.T) {
	day := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.AddDate(0, 2, 0)}, -1)
	if err := fixture.db.Where("channel_id = ?", fixture.ch.ID).Delete(&storage.ChannelUsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.beforeRead = func() {
		if len(fixture.reads) > 2 {
			fixture.failRead = true
		}
	}
	recorder := &usageProgressRecorder{}
	ctx := progress.WithObserver(context.Background(), recorder)
	report, err := fixture.service.RefetchUsageHistoryWithReport(ctx, &fixture.ch, day.AddDate(0, -1, 0), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("upstream rejection was ignored")
	}
	count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", time.Time{}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if count != 1 || report.SavedRows != 1 || report.Result == nil || report.Result.LocalCount != 1 {
		t.Fatalf("completed repair was lost: count=%d report=%#v", count, report)
	}
	fixture.rejectCounts = false
	fixture.failRead = false
	fixture.beforeRead = nil
	fixture.service.usageLimiter = connector.NewUsageLimiter(0, time.Millisecond)
	fixture.reads = nil
	if err := fixture.service.RefetchUsageHistory(ctx, &fixture.ch, day.AddDate(0, -1, 0), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.reads) != 2 || fixture.reads[0].Since.Before(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("retry downloaded repaired history: %#v", fixture.reads)
	}
}

func TestUsageTotalsDoesNotExpandHistoricalDifferences(t *testing.T) {
	day := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.AddDate(0, 2, 0)}, 0)
	result, err := fixture.service.CheckUsageTotals(context.Background(), &fixture.ch, day.AddDate(0, -1, 0), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || result.Status != "mismatch" || result.UpstreamCount == nil || *result.UpstreamCount != 2 {
		t.Fatalf("totals failed: %#v err=%v", result, err)
	}
	if fixture.countRequests != 2 || len(fixture.reads) != 0 || result.Audit.CheckedWindows != 1 {
		t.Fatalf("page inspection drilled into history: counts=%d report=%#v", fixture.countRequests, result.Audit)
	}
}

func TestUsageRepairRefreshesCachedSegmentsWhenNewTotalConflicts(t *testing.T) {
	day := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fixture := newUsageHistoryFixture(t, []time.Time{day, day.AddDate(0, 2, 0)}, -1)
	if err := fixture.db.Where("channel_id = ?", fixture.ch.ID).Delete(&storage.ChannelUsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.beforeRead = func() { fixture.rejectCounts = true }
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	_ = fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, day.AddDate(0, -1, 0), until)
	fixture.rejectCounts = false
	fixture.beforeRead = nil
	fixture.service.usageLimiter = connector.NewUsageLimiter(0, time.Millisecond)
	fixture.remote = append(fixture.remote, day.Add(time.Hour))
	if err := fixture.service.RefetchUsageHistory(context.Background(), &fixture.ch, day.AddDate(0, -1, 0), until); err != nil {
		t.Fatal(err)
	}
	count, _ := fixture.repo.CountWindow(fixture.ch.ID, fixture.account.ID, "newapi", time.Time{}, until)
	if count != 3 {
		t.Fatalf("late history was hidden by cached segment: %d", count)
	}
}
