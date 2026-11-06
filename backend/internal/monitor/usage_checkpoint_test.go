package monitor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

type checkpointReader struct {
	pages    map[int]*connector.UsagePage
	queries  []connector.UsageQuery
	failPage int
	before   func(int)
}

func TestUsageCheckpointRejectsReorderedOverlappingSameTimestampPages(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, nil, -1)
	until := stamp.Add(time.Hour)
	if err := f.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: f.ch.ID, ChannelAccountID: f.account.ID, Source: "newapi", RemoteUsageID: "C", OccurredAt: stamp}}); err != nil {
		t.Fatal(err)
	}
	pages := make(map[int]*connector.UsagePage)
	for page, name := range map[int]string{1: "A", 4: "D", 3: "B", 2: "A"} {
		p := checkpointPage(page, stamp)
		p.Records[0].RemoteID = name
		p.Records[0].PageIdentity = name
		pages[page] = p
	}
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = &checkpointReader{pages: pages}
	if err := job.resumeHistory(context.Background(), until, 100); err == nil {
		t.Fatal("overlapping pages were treated as complete history")
	}
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(stamp) {
		t.Fatalf("overlapping pages advanced continuous boundary: %#v", state)
	}
	var retained int64
	f.db.Model(&storage.ChannelUsageEvent{}).Where("remote_usage_id = ?", "C").Count(&retained)
	if retained != 1 {
		t.Fatal("unverified same-timestamp row was removed")
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", stamp, until)
	if n != 4 {
		t.Fatalf("repeated upstream row was counted as an additional call: %d", n)
	}
}

func TestUsageCheckpointRejectsOutOfWindowPageWithoutClearingLocalData(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, nil, -1)
	until := stamp.Add(time.Hour)
	f.repo.SaveSyncState(&storage.ChannelUsageSyncState{ChannelID: f.ch.ID, ChannelAccountID: f.account.ID, HistoryVerifiedUntil: &stamp})
	f.repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: f.ch.ID, ChannelAccountID: f.account.ID, Source: "newapi", RemoteUsageID: "inside", OccurredAt: stamp.Add(time.Minute)}})
	p := checkpointPage(1, until.Add(time.Second))
	p.Total = 1
	p.Pages = 1
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = &checkpointReader{pages: map[int]*connector.UsagePage{1: p}}
	if err := job.resumeHistory(context.Background(), until, 100); err == nil {
		t.Fatal("out-of-window response was accepted")
	}
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(stamp) {
		t.Fatal("out-of-window response advanced checkpoint")
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", stamp, until)
	if n != 1 {
		t.Fatal("out-of-window response cleared local history")
	}
}

func TestUsageCheckpointPageTransactionDoesNotAdvanceOnFailure(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, nil, -1)
	r := &checkpointReader{pages: map[int]*connector.UsagePage{1: checkpointPage(1, stamp.Add(72*time.Hour)), 4: checkpointPage(4, stamp)}}
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	r.before = func(page int) {
		if page == 4 {
			f.db.Callback().Update().Before("gorm:update").Register("fail_history_state", func(tx *gorm.DB) {
				if tx.Statement.Table == "channel_usage_sync_states" {
					tx.AddError(errors.New("simulated commit failure"))
				}
			})
		}
	}
	if err := job.resumeHistory(context.Background(), stamp.Add(96*time.Hour), 2); err == nil {
		t.Fatal("commit failure was ignored")
	}
	f.db.Callback().Update().Remove("fail_history_state")
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil != nil {
		t.Fatal("failed transaction advanced checkpoint")
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", time.Time{}, stamp.Add(96*time.Hour))
	if n != 1 {
		t.Fatalf("failed page did not roll back atomically: %d", n)
	}
	r.before = nil
	r.queries = nil
	job, _ = f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	if err := job.resumeHistory(context.Background(), stamp.Add(96*time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	if len(r.queries) != 1 || r.queries[0].Page != 4 {
		t.Fatalf("failed page was skipped: %#v", r.queries)
	}
}

func TestUsageCheckpointChangedPaginationRetainsRowsAndRestartsAtBoundary(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, nil, -1)
	r := &checkpointReader{pages: map[int]*connector.UsagePage{1: checkpointPage(1, stamp.Add(72*time.Hour)), 4: checkpointPage(4, stamp), 3: checkpointPage(3, stamp.Add(24*time.Hour))}}
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	until := stamp.Add(96 * time.Hour)
	if err := job.resumeHistory(context.Background(), until, 2); err != nil {
		t.Fatal(err)
	}
	r.pages[3].Total = 5
	r.pages[3].Pages = 5
	if err := job.resumeHistory(context.Background(), until, 1); err == nil {
		t.Fatal("pagination change was ignored")
	}
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(stamp) || state.HistoryResume != "" {
		t.Fatalf("changed pagination skipped history: %#v", state)
	}
	var n int64
	f.db.Model(&storage.ChannelUsageEvent{}).Count(&n)
	if n != 3 {
		t.Fatalf("received rows were lost on pagination change: %d", n)
	}
	r.pages[1] = checkpointPage(1, stamp.Add(72*time.Hour))
	r.queries = nil
	job, _ = f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	if err := job.resumeHistory(context.Background(), until, 1); err != nil {
		t.Fatal(err)
	}
	if len(r.queries) != 1 || !r.queries[0].Since.Equal(stamp) {
		t.Fatalf("restarted from beginning: %#v", r.queries)
	}
}

func TestUsageCheckpointCustomRangeDoesNotAdvanceFullHistory(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, nil, -1)
	page := checkpointPage(1, stamp)
	page.Total = 1
	page.Pages = 1
	page.PageSize = 100
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = &checkpointReader{pages: map[int]*connector.UsagePage{1: page}}
	if err := job.readWindow(context.Background(), usageHistoryWindow{Since: stamp, Until: stamp.Add(time.Hour)}, 1, true); err != nil {
		t.Fatal(err)
	}
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil != nil {
		t.Fatal("custom range jumped across unchecked history")
	}
}

func (r *checkpointReader) ListUsage(_ context.Context, _ *connector.Channel, _ *connector.AuthSession, q connector.UsageQuery) (*connector.UsagePage, error) {
	r.queries = append(r.queries, q)
	if r.before != nil {
		r.before(q.Page)
	}
	if q.Page == r.failPage {
		return nil, errors.New("HTTP 429")
	}
	p := r.pages[q.Page]
	if p == nil {
		return nil, fmt.Errorf("unexpected page %d", q.Page)
	}
	return p, nil
}

func checkpointPage(page int, stamps ...time.Time) *connector.UsagePage {
	p := &connector.UsagePage{TotalKnown: true, Total: 4, Pages: 4, Page: page, PageSize: 1, RawCount: len(stamps), RawTimes: stamps}
	for _, stamp := range stamps {
		p.Records = append(p.Records, connector.UsageRecord{RemoteID: fmt.Sprintf("newapi:v2:%d", stamp.Unix()), PageIdentity: fmt.Sprintf("row-%d-%d", page, stamp.Unix()), CreatedAt: stamp, ActualCost: 2})
	}
	return p
}

func TestUsageCheckpointSavesEachPageAndResumesAfterRestart(t *testing.T) {
	start := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	day20 := start.AddDate(0, 0, 5)
	day25 := start.AddDate(0, 0, 10)
	latest := start.AddDate(0, 0, 20)
	until := latest.Add(24 * time.Hour)
	f := newUsageHistoryFixture(t, nil, -1)
	r := &checkpointReader{pages: map[int]*connector.UsagePage{1: checkpointPage(1, latest), 4: checkpointPage(4, start), 3: checkpointPage(3, day20), 2: checkpointPage(2, day25)}, failPage: 2}
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	r.before = func(page int) {
		if page == 4 {
			n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", time.Time{}, until)
			if n != 1 {
				t.Fatalf("first page was not committed before next request: %d", n)
			}
		}
	}
	if err := job.resumeHistory(context.Background(), until, 100); err == nil {
		t.Fatal("expected rejection")
	}
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(day20) {
		t.Fatalf("wrong contiguous boundary: %#v", state)
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", time.Time{}, until)
	if n != 3 {
		t.Fatalf("partial pages were discarded: %d", n)
	}
	forward := latest.Add(12 * time.Hour)
	state.ForwardSyncedAt = &forward
	if err := f.repo.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}
	r.failPage = 0
	r.queries = nil
	r.before = nil
	job, _ = f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	if err := job.resumeHistory(context.Background(), until, 100); err != nil {
		t.Fatal(err)
	}
	if len(r.queries) != 1 || r.queries[0].Page != 2 {
		t.Fatalf("re-read completed pages: %#v", r.queries)
	}
	state, _ = f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(until) {
		t.Fatalf("did not finish continuous range: %#v", state)
	}
}

func TestUsageCheckpointDoesNotSkipSameTimestampAcrossPages(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := newUsageHistoryFixture(t, nil, -1)
	r := &checkpointReader{pages: map[int]*connector.UsagePage{1: checkpointPage(1, stamp.Add(24*time.Hour)), 4: checkpointPage(4, stamp), 3: checkpointPage(3, stamp), 2: checkpointPage(2, stamp)}, failPage: 2}
	job, _ := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	until := stamp.Add(48 * time.Hour)
	_ = job.resumeHistory(context.Background(), until, 100)
	state, _ := f.repo.SyncState(f.ch.ID, f.account.ID)
	if state.HistoryVerifiedUntil == nil || !state.HistoryVerifiedUntil.Equal(stamp) {
		t.Fatalf("same-timestamp group was skipped: %#v", state)
	}
	r.failPage = 0
	job, _ = f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	job.reader = r
	if err := job.resumeHistory(context.Background(), until, 100); err != nil {
		t.Fatal(err)
	}
	n, _ := f.repo.CountWindow(f.ch.ID, f.account.ID, "newapi", time.Time{}, until)
	if n != 4 {
		t.Fatalf("identical real calls were collapsed across pages: %d", n)
	}
}
