package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/progress"
)

var errUsageScanBudget = errors.New("historical usage page budget reached; saved the checkpoint for the next run")

type newAPIUsageRange struct {
	Since, Until time.Time
	Level        string
}
type newAPIUsagePeriod struct {
	Raw, Calls, Retained int64
	Oldest               time.Time
	IdentityCounts       map[string]int
}
type newAPIUsagePage struct {
	ID                string
	NextPage          int
	PageSize, Pages   int
	Total, Raw, Calls int64
	Oldest, Last      time.Time
	IdentityCounts    map[string]int
}
type newAPIUsagePlan struct {
	Since, Until    time.Time
	Oldest          time.Time
	Full            bool
	Queue           []newAPIUsageRange
	Active          *newAPIUsagePage
	Calls, Retained int64
}
type newAPIUsageScan struct {
	Origin, Probe                     time.Time
	ProbeStep                         int
	Periods                           map[string]newAPIUsagePeriod
	Plans                             map[string]*newAPIUsagePlan
	activePlan                        *newAPIUsagePlan
	CompletedCalls, CompletedRetained int64
}

func (job *usageHistoryAccount) newAPIScanState() (*newAPIUsageScan, error) {
	scan := &newAPIUsageScan{Periods: map[string]newAPIUsagePeriod{}}
	if job.state.HistoryScan != "" {
		if err := json.Unmarshal([]byte(job.state.HistoryScan), scan); err != nil {
			return nil, fmt.Errorf("invalid New API history checkpoint: %w", err)
		}
	}
	if scan.Periods == nil {
		scan.Periods = map[string]newAPIUsagePeriod{}
	}
	if scan.Plans == nil {
		scan.Plans = map[string]*newAPIUsagePlan{}
	}
	return scan, nil
}

func (job *usageHistoryAccount) saveNewAPIScan(scan *newAPIUsageScan) error {
	data, err := json.Marshal(scan)
	if err != nil {
		return err
	}
	job.state.HistoryScan = string(data)
	return nil
}

func newAPIUsageRangeKey(window newAPIUsageRange) string {
	return window.Since.Format(time.RFC3339) + "/" + window.Until.Format(time.RFC3339)
}

func usageIdentityCounts(records []connector.UsageRecord, since, until time.Time) map[string]int {
	counts := make(map[string]int)
	for _, record := range records {
		if record.RemoteID != "" && !record.CreatedAt.Before(since) && record.CreatedAt.Before(until) {
			counts[record.RemoteID]++
		}
	}
	return counts
}

func sameUsageIdentityCounts(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, count := range left {
		if right[key] != count {
			return false
		}
	}
	return true
}

func canonicalStoredUsageIDs(values map[string]int) map[string]int {
	result := make(map[string]int, len(values))
	for value, count := range values {
		base := value
		if index := strings.LastIndex(value, ":"); index > 0 && strings.HasPrefix(value, "newapi:v2:") {
			if _, err := strconv.Atoi(value[index+1:]); err == nil {
				base = value[:index]
			}
		}
		result[base] += count
	}
	return result
}

func newAPIUsageChildren(window newAPIUsageRange, total int64) []newAPIUsageRange {
	level := ""
	if total == 2000 {
		switch window.Level {
		case "month":
			level = "week"
		case "week":
			level = "day"
		case "day":
			level = "hour"
		}
	} else if total > 10000 && window.Level == "month" {
		level = "week"
	}
	if level == "" {
		return nil
	}
	var result []newAPIUsageRange
	if level == "hour" {
		for cursor := window.Since; cursor.Before(window.Until); {
			next := cursor.Truncate(time.Hour).Add(time.Hour)
			if next.After(window.Until) {
				next = window.Until
			}
			result = append(result, newAPIUsageRange{Since: cursor, Until: next, Level: level})
			cursor = next
		}
	} else {
		for _, part := range splitUsageHistoryWindow(usageHistoryWindow{Since: window.Since, Until: window.Until}, level) {
			result = append(result, newAPIUsageRange{Since: part.Since, Until: part.Until, Level: level})
		}
	}
	return result
}

func (job *usageHistoryAccount) saveNewAPIPage(scan *newAPIUsageScan, runID string, page *connector.UsagePage, window newAPIUsageRange, done, clean bool, report *UsageHistoryReport) error {
	events := job.historyPageEvents(page.Records, window.Since, window.Until)
	var identities []string
	for _, record := range page.Records {
		if record.RemoteID == "" || record.CreatedAt.IsZero() || record.CreatedAt.Before(window.Since) || !record.CreatedAt.Before(window.Until) {
			continue
		}
		id := record.PageIdentity
		if id == "" {
			id = record.RemoteID
		}
		identities = append(identities, id)
	}
	since, until := time.Time{}, time.Time{}
	if clean {
		since, until = window.Since, window.Until
	}
	if err := job.saveNewAPIScan(scan); err != nil {
		return err
	}
	if err := job.service.channelUsage.CommitHistoryPage(job.state, runID, string(job.channel.Type), events, identities, since, until, done); err != nil {
		return err
	}
	report.SavedPages++
	report.SavedRows += len(events)
	if job.onHistoryPage != nil {
		job.onHistoryPage(page.Page, len(events))
	}
	return nil
}

func (job *usageHistoryAccount) discoverNewAPIOrigin(ctx context.Context, scan *newAPIUsageScan, until time.Time, budget *int, report *UsageHistoryReport) error {
	if !scan.Origin.IsZero() {
		return nil
	}
	if scan.Probe.IsZero() {
		var hint time.Time
		if reader, ok := job.reader.(connector.UsageHistoryOriginReader); ok {
			var err error
			hint, err = reader.UsageHistoryOrigin(ctx, job.resolved, job.session)
			if err != nil {
				return err
			}
		}
		local, err := job.service.channelUsage.EarliestOccurredAt(job.channel.ID, job.account.ID, string(job.channel.Type))
		if err != nil {
			return err
		}
		if local != nil && (hint.IsZero() || local.Before(hint)) {
			hint = *local
		}
		if hint.IsZero() || hint.After(until) {
			hint = until
		}
		scan.Probe = hint.UTC().Truncate(time.Second)
		scan.ProbeStep = 1
		if err := job.saveNewAPIScan(scan); err != nil {
			return err
		}
		if err := job.service.channelUsage.SaveSyncState(job.state); err != nil {
			return err
		}
	}
	for scan.Origin.IsZero() && *budget > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		progress.Start(ctx, progress.StageUsage, fmt.Sprintf("%s: \u6b63\u5728\u786e\u8ba4\u8d26\u53f7\u5386\u53f2\u8d77\u70b9", job.account.Username))
		page, err := job.reader.ListUsage(ctx, job.resolved, job.session, connector.UsageQuery{Until: scan.Probe, Page: 1, PageSize: 100})
		*budget--
		if err != nil {
			return err
		}
		if page == nil || !page.TotalKnown || len(page.RawTimes) != page.RawCount {
			return fmt.Errorf("upstream did not provide reliable history origin data")
		}
		window := newAPIUsageRange{Until: scan.Probe}
		if page.RawCount == 0 {
			if page.Total != 0 {
				return fmt.Errorf("upstream returned an incomplete history origin page")
			}
			scan.Origin = scan.Probe
		} else {
			oldest := page.RawTimes[len(page.RawTimes)-1]
			if oldest.IsZero() || !oldest.Before(scan.Probe) {
				return fmt.Errorf("upstream ignored the history origin time filter")
			}
			scan.Probe = time.Date(oldest.Year(), oldest.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -scan.ProbeStep, 0)
			if scan.ProbeStep < 120 {
				scan.ProbeStep *= 2
			}
			if scan.Probe.Before(time.Unix(0, 0)) {
				scan.Probe = time.Unix(0, 0).UTC()
				scan.Origin = scan.Probe
			}
		}
		if err := job.saveNewAPIPage(scan, fmt.Sprintf("probe-%d-%d-%d", job.channel.ID, job.account.ID, time.Now().UnixNano()), page, window, true, false, report); err != nil {
			return err
		}
	}
	if scan.Origin.IsZero() {
		return errUsageScanBudget
	}
	return nil
}

func (job *usageHistoryAccount) scanNewAPIHistory(ctx context.Context, since, until time.Time, budget int, report *UsageHistoryReport) (retErr error) {
	return job.scanNewAPIHistoryScope(ctx, since, until, budget, report, "history")
}

func (job *usageHistoryAccount) scanNewAPIHistoryScope(ctx context.Context, since, until time.Time, budget int, report *UsageHistoryReport, scope string) (retErr error) {
	defer func() {
		if retErr != nil {
			if state, err := job.service.channelUsage.SyncState(job.channel.ID, job.account.ID); err == nil {
				job.state = state
			}
		}
	}()
	if job.reader == nil {
		if err := job.ensurePrepared(ctx); err != nil {
			return err
		}
	}
	scan, err := job.newAPIScanState()
	if err != nil {
		return err
	}
	full := since.IsZero()
	if full {
		scope = "full"
	}
	if full {
		if err := job.discoverNewAPIOrigin(ctx, scan, until, &budget, report); err != nil {
			return err
		}
		since = scan.Origin
	}
	plan := scan.Plans[scope]
	if plan == nil || plan.Full != full || (!full && scope != "recent" && (!plan.Since.Equal(since) || !plan.Until.Equal(until))) {
		if plan != nil && plan.Active != nil {
			if err := job.service.channelUsage.ClearHistorySeen(plan.Active.ID); err != nil {
				return err
			}
		}
		plan = &newAPIUsagePlan{Since: since, Until: until, Full: full}
		for _, part := range splitUsageHistoryWindow(usageHistoryWindow{Since: since, Until: until}, "month") {
			plan.Queue = append(plan.Queue, newAPIUsageRange{Since: part.Since, Until: part.Until, Level: "month"})
		}
		scan.Plans[scope] = plan
		if full {
			boundary := since
			job.state.HistoryVerifiedUntil = &boundary
		}
	}
	scan.activePlan = plan
	if (full || scope == "recent") && plan.Until.Before(until) {
		for _, part := range splitUsageHistoryWindow(usageHistoryWindow{Since: plan.Until, Until: until}, "month") {
			plan.Queue = append(plan.Queue, newAPIUsageRange{Since: part.Since, Until: part.Until, Level: "month"})
		}
		plan.Until = until
	}
	if err := job.saveNewAPIScan(scan); err != nil {
		return err
	}
	if err := job.service.channelUsage.SaveSyncState(job.state); err != nil {
		return err
	}
	for len(plan.Queue) > 0 && budget > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		window := plan.Queue[0]
		if plan.Active == nil {
			plan.Active = &newAPIUsagePage{ID: fmt.Sprintf("napi-%d-%d-%d", job.channel.ID, job.account.ID, time.Now().UnixNano()), NextPage: 1, IdentityCounts: map[string]int{}}
		}
		run := plan.Active
		item := UsageWindowIntegrity{AccountID: job.account.ID, AccountName: job.account.Username, Since: window.Since, Until: window.Until, Level: window.Level, Status: "checking"}
		progress.Start(ctx, progress.StageUsage, usageHistoryWindowMessage(item)+fmt.Sprintf("\uff0c\u7b2c %d \u9875", run.NextPage))
		page, err := job.reader.ListUsage(ctx, job.resolved, job.session, connector.UsageQuery{Since: window.Since, Until: window.Until, Page: run.NextPage, PageSize: 100})
		budget--
		if err != nil {
			return err
		}
		if page == nil {
			return fmt.Errorf("upstream returned an empty usage page response")
		}
		validation := usageHistoryResume{Since: window.Since, Until: window.Until, Source: string(job.channel.Type)}
		if run.NextPage > 1 {
			validation.NextPage = run.NextPage
			validation.Total = run.Total
			validation.PageSize = run.PageSize
			validation.Pages = run.Pages
		}
		if err := validateHistoryPage(page, run.NextPage, &validation); err != nil {
			plan.Active = nil
			if saveErr := job.saveNewAPIPage(scan, run.ID, page, window, true, false, report); saveErr != nil {
				return saveErr
			}
			return err
		}
		local, err := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, string(job.channel.Type), window.Since, window.Until)
		if err != nil {
			return err
		}
		item.LocalCount = local
		item.UpstreamCount = page.Total
		item.TotalCapped = page.Total == 2000
		if len(page.RawTimes) > 0 {
			oldest := page.RawTimes[len(page.RawTimes)-1]
			if plan.Oldest.IsZero() || oldest.Before(plan.Oldest) {
				plan.Oldest = oldest
			}
		}
		if run.NextPage == 1 {
			run.PageSize = page.PageSize
			if run.PageSize <= 0 {
				run.PageSize = 100
			}
			run.Pages = page.Pages
			if run.Pages <= 0 {
				run.Pages = int((page.Total + int64(run.PageSize) - 1) / int64(run.PageSize))
				if run.Pages <= 0 {
					run.Pages = 1
				}
			}
			report.CheckedWindows++
			children := newAPIUsageChildren(window, page.Total)
			for len(children) == 1 && children[0].Since.Equal(window.Since) && children[0].Until.Equal(window.Until) {
				window = children[0]
				plan.Queue[0] = window
				item.Level = window.Level
				children = newAPIUsageChildren(window, page.Total)
			}
			if len(children) > 0 {
				item.Status = "subdividing"
				plan.Queue = append(children, plan.Queue[1:]...)
				plan.Active = nil
				if err := job.saveNewAPIPage(scan, run.ID, page, window, true, false, report); err != nil {
					return err
				}
				job.recordNewAPIWindow(ctx, report, item)
				continue
			}
			if page.Total == 2000 {
				item.Status = "capped"
				job.recordNewAPIWindow(ctx, report, item)
				if err := job.saveNewAPIPage(scan, run.ID, page, window, true, false, report); err != nil {
					return err
				}
				return fmt.Errorf("\u4e0a\u6e38\u5728\u5c0f\u65f6\u8303\u56f4\u4ecd\u8fd4\u56de 2000 \u6761\u4e0a\u9650\uff0c\u5df2\u4fdd\u5b58\u6536\u5230\u7684\u6570\u636e\uff0c\u8be5\u65f6\u6bb5\u5c1a\u672a\u5b8c\u6574\u6838\u5bf9")
			}
			key := newAPIUsageRangeKey(window)
			known, ok := scan.Periods[key]
			matched := ok && known.Raw == page.Total && local == known.Calls+known.Retained
			if ok && page.Total <= 100 && len(known.IdentityCounts) > 0 {
				localIDs, identityErr := job.service.channelUsage.UsageIdentityCounts(job.channel.ID, job.account.ID, string(job.channel.Type), window.Since, window.Until)
				if identityErr != nil {
					return identityErr
				}
				matched = sameUsageIdentityCounts(known.IdentityCounts, canonicalStoredUsageIDs(localIDs)) && sameUsageIdentityCounts(known.IdentityCounts, usageIdentityCounts(page.Records, window.Since, window.Until))
			}
			if page.Total <= 100 {
				known = newAPIUsagePeriod{Raw: page.Total, Calls: int64(len(page.Records))}
				if len(page.RawTimes) > 0 {
					known.Oldest = page.RawTimes[len(page.RawTimes)-1]
				}
				matched = local == known.Calls
			}
			if matched {
				report.SkippedWindows++
				item.Status = "matched"
				item.UpstreamCount = known.Calls
				job.finishNewAPIWindow(scan, window, known)
				if err := job.saveNewAPIScan(scan); err != nil {
					return err
				}
				if err := job.service.channelUsage.CommitHistoryPage(job.state, run.ID, string(job.channel.Type), nil, nil, time.Time{}, time.Time{}, true); err != nil {
					return err
				}
				if job.onHistoryPage != nil {
					job.onHistoryPage(page.Page, 0)
				}
				job.recordNewAPIWindow(ctx, report, item)
				continue
			}
			run.Total = page.Total
		}
		identities := make([]string, 0, len(page.Records))
		for _, record := range page.Records {
			identity := record.PageIdentity
			if identity == "" {
				identity = record.RemoteID
			}
			identities = append(identities, identity)
		}
		overlaps, err := job.service.channelUsage.HistoryPageOverlaps(run.ID, identities)
		if err != nil {
			return err
		}
		if overlaps {
			plan.Active = nil
			if err := job.saveNewAPIPage(scan, run.ID, page, window, true, false, report); err != nil {
				return err
			}
			return fmt.Errorf("upstream usage pages overlap; saved rows and retained the continuous checkpoint")
		}
		if len(page.RawTimes) > 0 {
			if !run.Last.IsZero() && page.RawTimes[0].After(run.Last) {
				return fmt.Errorf("upstream usage order changed; retained the checkpoint")
			}
			run.Last = page.RawTimes[len(page.RawTimes)-1]
			run.Oldest = run.Last
		}
		for _, record := range page.Records {
			if record.RemoteID != "" && !record.CreatedAt.Before(window.Since) && record.CreatedAt.Before(window.Until) {
				run.IdentityCounts[record.RemoteID]++
			}
		}
		run.Raw += int64(page.RawCount)
		run.Calls += int64(len(page.Records))
		run.NextPage++
		done := run.Raw == run.Total
		if run.Raw > run.Total {
			return fmt.Errorf("upstream returned more rows than its window total")
		}
		item.Status = "repairing"
		clean := false
		savedWindow := window
		if done {
			period := newAPIUsagePeriod{Raw: run.Total, Calls: run.Calls, Oldest: run.Oldest, IdentityCounts: run.IdentityCounts}
			clean = run.Total > 0
			if run.Total == 0 {
				period.Retained = local
			}
			if full && plan.Calls == 0 && !run.Oldest.IsZero() && run.Oldest.After(window.Since) {
				period.Retained, err = job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, string(job.channel.Type), window.Since, run.Oldest)
				if err != nil {
					return err
				}
				savedWindow.Since = run.Oldest
			}
			job.finishNewAPIWindow(scan, window, period)
			item.Status = "repaired"
			item.UpstreamCount = run.Calls
		}
		if err := job.saveNewAPIPage(scan, run.ID, page, savedWindow, done, clean, report); err != nil {
			return err
		}
		job.recordNewAPIWindow(ctx, report, item)
	}
	if len(plan.Queue) > 0 {
		return errUsageScanBudget
	}
	scan.CompletedCalls, scan.CompletedRetained = plan.Calls, plan.Retained
	if full {
		older, countErr := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, string(job.channel.Type), time.Time{}, plan.Since)
		if countErr != nil {
			return countErr
		}
		scan.CompletedRetained += older
	}
	delete(scan.Plans, scope)
	if full {
		now := time.Now().UTC()
		available := plan.Oldest
		if available.IsZero() {
			available = plan.Until
		}
		job.state.HistoryAvailableSince = &available
		job.state.HistoryCheckedAt = &now
		job.state.BackfillVersion = channelUsageBackfillVersion
		job.state.HistoryResume = ""
		job.state.HistoryAuditBefore = nil
		job.state.HistoryAuditRemaining = nil
	}
	if err := job.saveNewAPIScan(scan); err != nil {
		return err
	}
	if err := job.service.channelUsage.SaveSyncState(job.state); err != nil {
		return err
	}
	return nil
}

func (job *usageHistoryAccount) finishNewAPIWindow(scan *newAPIUsageScan, window newAPIUsageRange, period newAPIUsagePeriod) {
	if len(scan.Periods) >= 256 {
		scan.Periods = map[string]newAPIUsagePeriod{}
	}
	scan.Periods[newAPIUsageRangeKey(window)] = period
	plan := scan.activePlan
	if !period.Oldest.IsZero() && (plan.Oldest.IsZero() || period.Oldest.Before(plan.Oldest)) {
		plan.Oldest = period.Oldest
	}
	plan.Calls += period.Calls
	plan.Retained += period.Retained
	plan.Queue = plan.Queue[1:]
	plan.Active = nil
	if plan.Full {
		boundary := window.Until
		job.state.HistoryVerifiedUntil = &boundary
	}
}

func (job *usageHistoryAccount) recordNewAPIWindow(ctx context.Context, report *UsageHistoryReport, item UsageWindowIntegrity) {
	for index := range report.Windows {
		existing := &report.Windows[index]
		if existing.AccountID == item.AccountID && existing.Level == item.Level && existing.Since.Equal(item.Since) && existing.Until.Equal(item.Until) {
			// 一个区间可能有多页。报告只保留一个区间，后续页更新该区间的最新状态。
			if item.Status == "repaired" || existing.Status != "repaired" {
				*existing = item
			} else {
				existing.LocalCount = item.LocalCount
				existing.UpstreamCount = item.UpstreamCount
				existing.TotalCapped = item.TotalCapped
			}
			progress.FromContext(ctx).Emit(progress.Event{Stage: progress.StageUsage, Message: usageHistoryWindowMessage(item), Data: map[string]any{"kind": "usage_window", "window": item, "saved_pages": report.SavedPages, "saved_rows": report.SavedRows}, Time: time.Now()})
			return
		}
	}
	if len(report.Windows) < 128 {
		report.Windows = append(report.Windows, item)
	}
	progress.FromContext(ctx).Emit(progress.Event{Stage: progress.StageUsage, Message: usageHistoryWindowMessage(item), Data: map[string]any{"kind": "usage_window", "window": item, "saved_pages": report.SavedPages, "saved_rows": report.SavedRows}, Time: time.Now()})
}
