package monitor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

type cappedUsageReader struct {
	rows             []time.Time
	queries          []connector.UsageQuery
	cap              int64
	responsePageSize int
	failAt           int
	before           func(connector.UsageQuery)
	origin           time.Time
}

func (r *cappedUsageReader) UsageHistoryOrigin(context.Context, *connector.Channel, *connector.AuthSession) (time.Time, error) {
	return r.origin, nil
}

func (r *cappedUsageReader) ListUsage(_ context.Context, _ *connector.Channel, _ *connector.AuthSession, q connector.UsageQuery) (*connector.UsagePage, error) {
	r.queries = append(r.queries, q)
	if r.before != nil {
		r.before(q)
	}
	if r.failAt > 0 && len(r.queries) == r.failAt {
		return nil, errors.New("rate limited")
	}
	var records []connector.UsageRecord
	for i := len(r.rows) - 1; i >= 0; i-- {
		stamp := r.rows[i]
		if !stamp.Before(q.Since) && stamp.Before(q.Until) {
			id := fmt.Sprintf("remote-%d", i)
			records = append(records, connector.UsageRecord{RemoteID: id, PageIdentity: id, CreatedAt: stamp, Model: "test"})
		}
	}
	total := int64(len(records))
	if r.cap > 0 && total > r.cap {
		total = r.cap
		records = records[:r.cap]
	}
	pageSize := q.PageSize
	if r.responsePageSize > 0 {
		pageSize = r.responsePageSize
	}
	start := (q.Page - 1) * pageSize
	end := start + pageSize
	if start > len(records) {
		start = len(records)
	}
	if end > len(records) {
		end = len(records)
	}
	out := &connector.UsagePage{TotalKnown: true, Total: total, Page: q.Page, PageSize: pageSize, Pages: int((total + int64(pageSize) - 1) / int64(pageSize)), Records: records[start:end], RawCount: end - start}
	for _, row := range out.Records {
		out.RawTimes = append(out.RawTimes, row.CreatedAt)
	}
	return out, nil
}

func newCappedHistoryJob(t *testing.T, rows []time.Time, cap int64) (*usageHistoryAccount, *cappedUsageReader) {
	t.Helper()
	f := newUsageHistoryFixture(t, nil, -1)
	job, err := f.service.prepareUsageAccount(context.Background(), &f.ch, f.account)
	if err != nil {
		t.Fatal(err)
	}
	reader := &cappedUsageReader{rows: rows, cap: cap}
	job.reader = reader
	return job, reader
}

func TestNewAPIHistorySplitsCappedTotalsAndCommitsEveryPage(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var rows []time.Time
	for i := 0; i < 2301; i++ {
		rows = append(rows, start.Add(time.Duration(i)*2*time.Minute))
	}
	job, reader := newCappedHistoryJob(t, rows, 2000)
	reader.before = func(q connector.UsageQuery) {
		if q.Page == 2 {
			n, _ := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, "newapi", start, start.AddDate(0, 1, 0))
			if n == 0 {
				t.Error("previous page was not saved before reading the next page")
			}
		}
	}
	report := &UsageHistoryReport{}
	if err := job.scanNewAPIHistory(context.Background(), start, start.AddDate(0, 1, 0), 200, report); err != nil {
		t.Fatal(err)
	}
	n, _ := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, "newapi", start, start.AddDate(0, 1, 0))
	if n != int64(len(rows)) {
		t.Fatalf("saved %d, want %d", n, len(rows))
	}
	levels := map[string]bool{}
	for _, item := range report.Windows {
		levels[item.Level] = true
	}
	if !levels["month"] || !levels["week"] || !levels["day"] {
		t.Fatalf("missing subdivision: %#v", levels)
	}
	seen := map[string]bool{}
	for _, q := range reader.queries {
		if q.PageSize != 100 {
			t.Fatalf("page size = %d", q.PageSize)
		}
		key := fmt.Sprintf("%s/%s/%d", q.Since, q.Until, q.Page)
		if seen[key] {
			t.Fatalf("page fetched twice: %s", key)
		}
		seen[key] = true
	}
}

func TestNewAPIHistoryCappedLocalEqualityStillSplitsToHours(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var rows []time.Time
	for i := 0; i < 2100; i++ {
		rows = append(rows, start.Add(time.Duration(i)*2*time.Second))
	}
	job, reader := newCappedHistoryJob(t, rows, 2000)
	if err := job.service.channelUsage.Upsert(job.historyPageEvents((&connector.UsagePage{Records: makeRecords(rows[:2000])}).Records, start, start.AddDate(0, 1, 0))); err != nil {
		t.Fatal(err)
	}
	report := &UsageHistoryReport{}
	if err := job.scanNewAPIHistory(context.Background(), start, start.AddDate(0, 1, 0), 300, report); err != nil {
		t.Fatal(err)
	}
	var hours bool
	for _, q := range reader.queries {
		if q.Until.Sub(q.Since) <= time.Hour {
			hours = true
		}
	}
	if !hours {
		t.Fatal("capped total was mistaken for an exact matching count")
	}
	n, _ := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, "newapi", start, start.AddDate(0, 1, 0))
	if n != 2100 {
		t.Fatalf("saved %d, want 2100", n)
	}
}

func makeRecords(rows []time.Time) []connector.UsageRecord {
	var result []connector.UsageRecord
	for i, stamp := range rows {
		id := fmt.Sprintf("remote-%d", i)
		result = append(result, connector.UsageRecord{RemoteID: id, PageIdentity: id, CreatedAt: stamp, Model: "test"})
	}
	return result
}

func TestNewAPIHistoryUncappedMonthAndDurableResume(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var rows []time.Time
	for i := 0; i < 2401; i++ {
		rows = append(rows, start.Add(time.Duration(i)*time.Minute))
	}
	job, reader := newCappedHistoryJob(t, rows, 0)
	reader.failAt = 4
	if err := job.scanNewAPIHistory(context.Background(), start, start.AddDate(0, 1, 0), 100, &UsageHistoryReport{}); err == nil {
		t.Fatal("expected interruption")
	}
	n, _ := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, "newapi", start, start.AddDate(0, 1, 0))
	if n != 300 {
		t.Fatalf("saved before interruption: %d", n)
	}
	state, err := job.service.channelUsage.SyncState(job.channel.ID, job.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	job.state = state
	reader.failAt = 0
	reader.queries = nil
	if err := job.scanNewAPIHistory(context.Background(), start, start.AddDate(0, 1, 0), 100, &UsageHistoryReport{}); err != nil {
		t.Fatal(err)
	}
	if reader.queries[0].Page != 4 {
		t.Fatalf("resumed page %d, want 4", reader.queries[0].Page)
	}
	for _, q := range reader.queries {
		if !q.Since.Equal(start) || !q.Until.Equal(start.AddDate(0, 1, 0)) {
			t.Fatal("uncapped month unnecessarily subdivided")
		}
	}
	n, _ = job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, "newapi", start, start.AddDate(0, 1, 0))
	if n != 2401 {
		t.Fatalf("saved after resume: %d", n)
	}
	reader.queries = nil
	if err := job.scanNewAPIHistory(context.Background(), start, start.AddDate(0, 1, 0), 100, &UsageHistoryReport{}); err != nil {
		t.Fatal(err)
	}
	if len(reader.queries) != 1 {
		t.Fatalf("healthy month used %d requests", len(reader.queries))
	}
}

func TestNewAPIHistoryFindsDeletedKeysOlderThanCreationHint(t *testing.T) {
	start := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	job, reader := newCappedHistoryJob(t, []time.Time{start, start.AddDate(0, 2, 0)}, 2000)
	reader.origin = start.AddDate(0, 1, 0)
	if err := job.scanNewAPIHistory(context.Background(), time.Time{}, start.AddDate(0, 3, 0), 100, &UsageHistoryReport{}); err != nil {
		t.Fatal(err)
	}
	n, _ := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, "newapi", time.Time{}, start.AddDate(0, 3, 0))
	if n != 2 {
		t.Fatalf("old key history lost: %d", n)
	}
	scan, err := job.newAPIScanState()
	if err != nil {
		t.Fatal(err)
	}
	if scan.Origin.After(start) {
		t.Fatalf("origin %s is newer than older key logs", scan.Origin)
	}
}

func TestNewAPIHistoryHourCapNeverAdvancesCheckpoint(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]time.Time, 2001)
	for i := range rows {
		rows[i] = start.Add(time.Duration(i) * time.Second)
	}
	job, reader := newCappedHistoryJob(t, rows, 2000)
	err := job.scanNewAPIHistory(context.Background(), start, start.Add(time.Hour), 100, &UsageHistoryReport{})
	if err == nil {
		t.Fatal("accepted a capped hour as complete")
	}
	if len(reader.queries) != 1 {
		t.Fatalf("identical narrowed windows fetched %d times", len(reader.queries))
	}
	n, _ := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, "newapi", start, start.Add(time.Hour))
	if n != 100 {
		t.Fatalf("received hour page not saved: %d", n)
	}
	if job.state.HistoryVerifiedUntil != nil {
		t.Fatal("capped hour advanced the continuous checkpoint")
	}
}

func TestNewAPIHistoryFullResumeNeverJumpsAcrossIncompleteMonth(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	job, reader := newCappedHistoryJob(t, []time.Time{start.Add(time.Hour), start.AddDate(0, 1, 0).Add(time.Hour)}, 0)
	scan, _ := job.newAPIScanState()
	scan.Origin = start
	if err := job.saveNewAPIScan(scan); err != nil {
		t.Fatal(err)
	}
	reader.failAt = 2
	if err := job.scanNewAPIHistory(context.Background(), time.Time{}, start.AddDate(0, 2, 0), 100, &UsageHistoryReport{}); err == nil {
		t.Fatal("expected interruption")
	}
	if job.state.HistoryVerifiedUntil == nil || !job.state.HistoryVerifiedUntil.Equal(start.AddDate(0, 1, 0)) {
		t.Fatalf("wrong continuous checkpoint: %v", job.state.HistoryVerifiedUntil)
	}
	reader.failAt = 0
	reader.queries = nil
	if err := job.scanNewAPIHistoryScope(context.Background(), start.AddDate(0, 1, 0), start.AddDate(0, 2, 0), 100, &UsageHistoryReport{}, "recent"); err != nil {
		t.Fatal(err)
	}
	if !job.state.HistoryVerifiedUntil.Equal(start.AddDate(0, 1, 0)) {
		t.Fatal("recent synchronization jumped over incomplete full history")
	}
	reader.queries = nil
	if err := job.scanNewAPIHistory(context.Background(), time.Time{}, start.AddDate(0, 2, 0), 100, &UsageHistoryReport{}); err != nil {
		t.Fatal(err)
	}
	if reader.queries[0].Since.Before(start.AddDate(0, 1, 0)) {
		t.Fatal("full history restarted before the continuous checkpoint")
	}
}

func TestNewAPIHistoryPersistsEffectivePageSizeForResume(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]time.Time, 251)
	for i := range rows {
		rows[i] = start.Add(time.Duration(i) * time.Minute)
	}
	job, reader := newCappedHistoryJob(t, rows, 0)
	reader.responsePageSize = 50
	reader.failAt = 3
	if err := job.scanNewAPIHistory(context.Background(), start, start.AddDate(0, 1, 0), 100, &UsageHistoryReport{}); err == nil {
		t.Fatal("expected page interruption")
	}
	reader.failAt = 0
	reader.queries = nil
	if err := job.scanNewAPIHistory(context.Background(), start, start.AddDate(0, 1, 0), 100, &UsageHistoryReport{}); err != nil {
		t.Fatal(err)
	}
	if len(reader.queries) == 0 || reader.queries[0].Page != 3 {
		t.Fatalf("resumed page=%d", reader.queries[0].Page)
	}
}

func TestNewAPIWindowReportDeduplicatesPages(t *testing.T) {
	job, _ := newCappedHistoryJob(t, nil, 0)
	report := &UsageHistoryReport{}
	window := UsageWindowIntegrity{AccountID: job.account.ID, AccountName: job.account.Username, Since: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), Level: "week", Status: "repairing", LocalCount: 100, UpstreamCount: 1388}
	job.recordNewAPIWindow(context.Background(), report, window)
	window.Status = "repaired"
	window.LocalCount = 1388
	job.recordNewAPIWindow(context.Background(), report, window)
	if len(report.Windows) != 1 {
		t.Fatalf("same weekly range was reported %d times", len(report.Windows))
	}
	if report.Windows[0].Status != "repaired" || report.Windows[0].LocalCount != 1388 {
		t.Fatalf("latest weekly status was not retained: %#v", report.Windows[0])
	}
}
