package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestContinuousHistoryRetainsOlderRowsWithoutReportingFailure(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, []time.Time{day}, 0)
	f.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: f.ch.ID, ChannelAccountID: f.account.ID, Source: "newapi", RemoteUsageID: "retained-old", OccurredAt: day.AddDate(0, -1, 0)}})
	report, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until)
	if err != nil {
		t.Fatalf("completed collection was rejected because older local history was retained: %v", err)
	}
	if report.Result.Status != "matched" || report.Result.Phase != "complete" {
		t.Fatalf("wrong completion result: %#v", report.Result)
	}
	if report.Result.RetainedCount != 1 || report.Result.Accounts[0].RetainedCount != 1 {
		t.Fatalf("retained history was not separated: %#v", report.Result)
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", time.Time{}, until)
	if n != 2 {
		t.Fatalf("old history was removed: %d", n)
	}
	f.reads = nil
	if _, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until); err != nil {
		t.Fatal(err)
	}
	if len(f.reads) != 2 {
		t.Fatalf("expected one probe for each historical month: %#v", f.reads)
	}
	check, err := f.service.CheckUsageTotals(context.Background(), &f.ch, time.Time{}, until)
	if err != nil || check.Status != "matched" {
		t.Fatalf("list still reports false mismatch: %#v %v", check, err)
	}
}

func TestContinuousHistoryAdoptsCoverageWithoutRewindingCompletedCheckpoint(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, []time.Time{day}, 0)
	f.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: f.ch.ID, ChannelAccountID: f.account.ID, Source: "newapi", RemoteUsageID: "retained-old", OccurredAt: day.AddDate(0, -1, 0)}})
	if _, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until); err != nil {
		state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
		if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(until) {
			t.Fatal(err)
		}
	}
	// 模拟旧版已完成断点，但尚未记录上游可核对范围。
	if err := f.db.Model(&storage.ChannelUsageSyncState{}).Where("channel_id = ? AND channel_account_id = ?", f.ch.ID, f.account.ID).Update("history_available_since", nil).Error; err != nil {
		t.Fatal(err)
	}
	f.reads = nil
	report, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until)
	if err != nil {
		t.Fatalf("old checkpoint could not adopt retained coverage: %v", err)
	}
	if report.Result.Status != "matched" {
		t.Fatalf("old checkpoint remains mismatched: %#v", report.Result)
	}
	if len(f.reads) != 2 {
		t.Fatalf("coverage discovery enumerated all history: %#v", f.reads)
	}
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(until) {
		t.Fatal("old continuous checkpoint was reset")
	}
}

func TestContinuousHistoryRepairsGenuineGapWithinAvailableCoverage(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, []time.Time{day, day.Add(24 * time.Hour)}, -1)
	if _, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Where("channel_id = ? AND occurred_at >= ?", f.ch.ID, day.Add(24*time.Hour)).Delete(&storage.ChannelUsageEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	f.reads = nil
	report, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until)
	if err != nil {
		t.Fatalf("existing checkpoint masked a real gap: %v", err)
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", time.Time{}, until)
	if n != 2 || report.Result.Status != "matched" {
		t.Fatalf("real gap was ignored: count=%d result=%#v", n, report.Result)
	}
}

func TestContinuousHistoryCoverageReusesSnapshotAndReadsOnlyFirstPage(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	stamps := make([]time.Time, 301)
	for i := range stamps {
		stamps[i] = day.Add(time.Duration(i) * time.Minute)
	}
	f := newUsageHistoryFixture(t, stamps, -1)
	if _, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: f.ch.ID, ChannelAccountID: f.account.ID, Source: "newapi", RemoteUsageID: "retained-old", OccurredAt: day.AddDate(0, -1, 0)}}); err != nil {
		t.Fatal(err)
	}
	f.db.Model(&storage.ChannelUsageSyncState{}).Where("channel_id = ?", f.ch.ID).Update("history_available_since", nil)
	f.reads = nil
	report, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.reads) != 1 || report.Result.RetainedCount != 1 {
		t.Fatalf("coverage detection enumerated history: reads=%d result=%#v", len(f.reads), report.Result)
	}
}

func TestContinuousHistoryRefreshesSummaryChangedDuringCollection(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, []time.Time{day, day.Add(time.Hour)}, -1)
	root := 1
	f.rootCount = &root
	f.beforeRead = func() { f.rootCount = nil }
	report, err := f.service.RefetchUsageHistoryWithReport(context.Background(), &f.ch, time.Time{}, until)
	if err != nil {
		t.Fatal(err)
	}
	if report.Result.UpstreamCount == nil || *report.Result.UpstreamCount != 2 || report.Result.Status != "matched" || len(f.reads) != 2 || f.countRequests != 0 {
		t.Fatalf("stale summary caused another page download: %#v reads=%d", report.Result, len(f.reads))
	}
}

func TestHistoryCoverageFailureDoesNotChangeTheExistingBoundary(t *testing.T) {
	day := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, nil, -1)
	until := day.AddDate(0, 0, 7)
	state := &storage.ChannelUsageSyncState{ChannelID: f.ch.ID, ChannelAccountID: f.account.ID, HistoryVerifiedUntil: &until}
	if err := f.repo.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	if err := job.ensurePrepared(context.Background()); err != nil {
		t.Fatal(err)
	}
	job.reader = &checkpointReader{pages: map[int]*connector.UsagePage{1: checkpointPage(1, day), 4: checkpointPage(4, day.Add(time.Hour))}}
	if err := job.discoverAvailableHistory(context.Background(), until); err == nil {
		t.Fatal("reversed coverage pages were accepted")
	}
	state, _ = f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryAvailableSince != nil || state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(until) {
		t.Fatal("unreliable coverage changed a durable boundary")
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", time.Time{}, until)
	if n != 2 {
		t.Fatal("received coverage pages were not saved before the failure")
	}
}
