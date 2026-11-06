package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func (job *usageHistoryAccount) retainedUsageCount(since, until time.Time) (int64, error) {
	if job.state.HistoryAvailableSince == nil || !since.Before(*job.state.HistoryAvailableSince) {
		return 0, nil
	}
	end := *job.state.HistoryAvailableSince
	if until.Before(end) {
		end = until
	}
	return job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, string(job.channel.Type), since, end)
}

func (job *usageHistoryAccount) refreshAvailableResult(result *UsageIntegrity, index int) error {
	account := &result.Accounts[index]
	var err error
	account.LocalCount, err = job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, string(job.channel.Type), result.Since, result.Until)
	if err != nil {
		return err
	}
	account.RetainedCount, err = job.retainedUsageCount(result.Since, result.Until)
	if err != nil {
		return err
	}
	account.HistoryVerifiedUntil = job.state.HistoryVerifiedUntil
	account.HistoryAvailableSince = job.state.HistoryAvailableSince
	result.LocalCount, err = job.service.channelUsage.CountWindow(job.channel.ID, 0, string(job.channel.Type), result.Since, result.Until)
	if err != nil {
		return err
	}
	refreshUsageIntegrityTotals(result)
	return nil
}

func (job *usageHistoryAccount) reconcileAvailableHistory(ctx context.Context, result *UsageIntegrity, index int, hadCoverage bool) error {
	if err := job.refreshAvailableResult(result, index); err != nil {
		return err
	}
	matches := func() bool {
		a := result.Accounts[index]
		return a.UpstreamCount != nil && a.LocalCount-a.RetainedCount == *a.UpstreamCount
	}
	if matches() {
		return nil
	}
	if err := job.ensurePrepared(ctx); err != nil {
		return err
	}
	remote, err := job.counter.CountUsage(ctx, job.resolved, job.session, connector.UsageQuery{Since: result.Since, Until: result.Until})
	if err != nil {
		return err
	}
	result.Accounts[index].UpstreamCount = &remote
	if err := job.refreshAvailableResult(result, index); err != nil {
		return err
	}
	if matches() {
		return nil
	}
	// 旧断点不回退，只用首尾页确认上游目前可核对的最早时间。
	if hadCoverage || job.state.HistoryAvailableSince == nil {
		if err := job.discoverAvailableHistory(ctx, result.Until); err != nil {
			return err
		}
		if err := job.refreshAvailableResult(result, index); err != nil {
			return err
		}
		if matches() {
			return nil
		}
	}
	if job.state.HistoryAvailableSince == nil || !job.state.HistoryAvailableSince.Before(result.Until) {
		return job.availableHistoryMismatch(result, index)
	}
	job.service.usageAudit.invalidate(job.channel.ID)
	window := usageHistoryWindow{Since: *job.state.HistoryAvailableSince, Until: result.Until}
	walker := job.walker(ctx, result.Audit, true)
	walker.Progress = func(item UsageWindowIntegrity) {
		phase := "locating"
		if item.Status == "repairing" || item.Status == "repaired" {
			phase = "repairing"
		}
		emitUsageIntegrity(ctx, result, phase, usageHistoryWindowMessage(item))
	}
	known := usageWindowCounts{Local: result.Accounts[index].LocalCount - result.Accounts[index].RetainedCount, Remote: remote}
	if _, err := walker.walk(ctx, window, "total", &known); err != nil {
		return err
	}
	if err := job.refreshAvailableResult(result, index); err != nil {
		return err
	}
	if !matches() {
		return job.availableHistoryMismatch(result, index)
	}
	return nil
}

func (job *usageHistoryAccount) availableHistoryMismatch(result *UsageIntegrity, index int) error {
	account := result.Accounts[index]
	upstream := "\u672a\u53d6\u5f97"
	if account.UpstreamCount != nil {
		upstream = fmt.Sprint(*account.UpstreamCount)
	}
	return fmt.Errorf("%s: \u53ef\u6838\u5bf9\u8303\u56f4\u4ecd\u6709\u5dee\u5f02\uff08\u672c\u5730 %d / \u4e0a\u6e38 %s\uff0c\u53e6\u4fdd\u7559 %d \u6761\u66f4\u65e9\u5386\u53f2\uff09\uff1b\u5df2\u4fdd\u5b58\u53d6\u5f97\u7684\u6570\u636e\u548c\u8fde\u7eed\u65ad\u70b9", account.AccountName, account.LocalCount-account.RetainedCount, upstream, account.RetainedCount)
}

func (job *usageHistoryAccount) discoverAvailableHistory(ctx context.Context, until time.Time) error {
	if err := job.ensurePrepared(ctx); err != nil {
		return err
	}
	run := usageHistoryResume{ID: fmt.Sprintf("coverage-%d-%d-%d", job.channel.ID, job.account.ID, time.Now().UnixNano()), Source: string(job.channel.Type), Until: until, PageSize: 100}
	defer job.service.channelUsage.ClearHistorySeen(run.ID)
	page := 1
	for {
		result, err := job.reader.ListUsage(ctx, job.resolved, job.session, connector.UsageQuery{Until: until, Page: page, PageSize: run.PageSize})
		if err != nil {
			return err
		}
		if result == nil {
			return fmt.Errorf("upstream returned an empty coverage page response")
		}
		events := job.historyPageEvents(result.Records, time.Time{}, until)
		identities := make([]string, 0, len(events))
		for _, record := range result.Records {
			if record.RemoteID == "" || record.CreatedAt.IsZero() || !record.CreatedAt.Before(until) {
				continue
			}
			id := record.PageIdentity
			if id == "" {
				id = record.RemoteID
			}
			identities = append(identities, id)
		}
		validErr := validateHistoryPage(result, page, &run)
		if err := job.service.channelUsage.CommitHistoryPage(job.state, run.ID, run.Source, events, identities, time.Time{}, time.Time{}, false); err != nil {
			return err
		}
		if job.onHistoryPage != nil {
			job.onHistoryPage(page, len(events))
		}
		if validErr != nil {
			return validErr
		}
		if page > 1 && len(result.RawTimes) > 0 && !run.FirstOldest.IsZero() && result.RawTimes[0].After(run.FirstOldest) {
			return fmt.Errorf("upstream coverage pages are not ordered from newest to oldest; retained the existing coverage boundary")
		}
		if page > 1 || result.Pages <= 1 {
			earliest := until
			if len(result.RawTimes) > 0 {
				earliest = result.RawTimes[len(result.RawTimes)-1]
			}
			job.state.HistoryAvailableSince = &earliest
			return job.service.channelUsage.SaveSyncState(job.state)
		}
		run.Total = result.Total
		run.Pages = result.Pages
		run.PageSize = result.PageSize
		run.NextPage = result.Pages
		if len(result.RawTimes) > 0 {
			run.FirstOldest = result.RawTimes[len(result.RawTimes)-1]
		}
		page = result.Pages
	}
}
