package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func (s *Service) lockUsageChannel(id uint) (func(), error) {
	value, _ := s.usageSyncMu.LoadOrStore(id, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	if !lock.TryLock() {
		return nil, fmt.Errorf("channel usage synchronization is already running")
	}
	return lock.Unlock, nil
}

func (s *Service) RefetchUsageHistoryWithReport(ctx context.Context, ch *storage.Channel, since, until time.Time) (*UsageHistoryReport, error) {
	report := &UsageHistoryReport{}
	if !since.Before(until) || until.After(time.Now().Add(time.Minute)) {
		return report, fmt.Errorf("invalid historical usage window")
	}
	if ch.BalanceMode == storage.BalanceModeManual || s.channelUsage == nil {
		return report, fmt.Errorf("channel does not expose upstream usage history")
	}
	if since.IsZero() {
		cutoff := time.Now().UTC().Add(-2 * time.Minute).Truncate(5 * time.Minute)
		if until.After(cutoff) {
			until = cutoff
		}
	}
	unlock, err := s.lockUsageChannel(ch.ID)
	if err != nil {
		return report, err
	}
	defer unlock()
	s.usageAudit.invalidateWindow(ch.ID, since, until)
	defer s.usageAudit.invalidateWindow(ch.ID, since, until)
	ctx = connector.WithUsageLimiter(ctx, s.usageLimiter)
	if ch.Type == storage.ChannelTypeNewAPI {
		return s.refetchNewAPIHistory(ctx, ch, since, until)
	}
	if since.IsZero() {
		return s.refetchContinuousHistory(ctx, ch, until)
	}
	result, repairs, err := s.checkUsageIntegrity(ctx, ch, since, until, true)
	if result.Audit != nil {
		report = result.Audit
	}
	report.Result = result
	if len(result.Accounts) == 0 && err == nil {
		err = fmt.Errorf("channel has no upstream accounts")
	}
	if err == nil {
		err = usageIntegrityFailure(result)
	}
	if err == nil && len(repairs) == 0 && result.Status == "mismatch" {
		err = fmt.Errorf("\u603b\u4f53\u6761\u6570\u4ecd\u6709\u5dee\u5f02\uff0c\u672a\u5b9a\u4f4d\u5230\u53ef\u4fee\u590d\u533a\u95f4")
	}
	if err == nil && result.Status == "mismatch" {
		err = fmt.Errorf("\u4fee\u590d\u540e\u603b\u4f53\u6761\u6570\u4ecd\u6709\u5dee\u5f02\uff0c\u8bf7\u91cd\u65b0\u6838\u5bf9")
	}
	if err != nil {
		result.Error = err.Error()
		s.emitUsageFailure(ctx, ch, result, "\u5904\u7406\u5df2\u4e2d\u65ad\uff0c\u5df2\u4fdd\u7559\u6838\u5bf9\u7ed3\u679c\u548c\u5df2\u5b8c\u6210\u7684\u4fee\u590d")
	} else {
		emitUsageIntegrity(ctx, result, "complete", "\u5386\u53f2\u6570\u636e\u6838\u5bf9\u4e0e\u4fee\u590d\u5b8c\u6210")
	}
	return report, err
}

func (job *usageHistoryAccount) auditRange(ctx context.Context, since, until time.Time, walker *usageHistoryWalker) (int64, error) {
	window := usageHistoryWindow{Since: since, Until: until}
	counts, err := job.checkTotal(ctx, window, walker)
	if err != nil {
		return 0, err
	}
	return job.auditKnownRange(ctx, window, walker, counts)
}

func (job *usageHistoryAccount) auditKnownRange(ctx context.Context, window usageHistoryWindow, walker *usageHistoryWalker, root usageWindowCounts) (int64, error) {
	if root.Local == root.Remote {
		return root.Remote, nil
	}
	if !window.Since.IsZero() {
		_, err := walker.walk(ctx, window, "total", &root)
		return root.Remote, err
	}
	counts := root
	if counts.Local == counts.Remote {
		return counts.Remote, nil
	}
	remaining := counts.Remote
	earliest, err := job.service.channelUsage.EarliestOccurredAt(job.channel.ID, job.account.ID, string(job.channel.Type))
	if err != nil {
		return counts.Remote, err
	}
	if remaining == 0 && earliest == nil {
		walker.Report.SkippedWindows++
		return 0, nil
	}
	var total int64
	for cursor := window.Until.UTC(); cursor.After(window.Since); {
		last := cursor.Add(-time.Nanosecond)
		start := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC)
		month := usageHistoryWindow{Since: start, Until: cursor}
		var known *usageWindowCounts
		if walker.Incremental {
			current, err := walker.readCounts(ctx, month, "month")
			if err != nil {
				return root.Remote, err
			}
			prefix, err := job.count(ctx, usageHistoryWindow{Until: start})
			if err != nil {
				return root.Remote, err
			}
			if current.Remote+prefix.Remote != remaining {
				return root.Remote, usageCountConflict("month", remaining, current.Remote+prefix.Remote)
			}
			known = &current
		}
		counts, err := walker.walk(ctx, month, "month", known)
		if err != nil {
			return root.Remote, err
		}
		total += counts.Remote
		cursor = start
		remaining -= counts.Remote
		if remaining < 0 {
			return root.Remote, usageCountConflict("month", root.Remote, total)
		}
		if remaining == 0 && (earliest == nil || !earliest.Before(cursor)) {
			return root.Remote, nil
		}
		if earliest == nil || !earliest.Before(cursor) {
			prefix, err := job.count(ctx, usageHistoryWindow{Until: cursor})
			if err != nil {
				return root.Remote, err
			}
			if prefix.Remote != remaining {
				return root.Remote, usageCountConflict("month", root.Remote, total+prefix.Remote)
			}
		}
	}
	if remaining != 0 {
		return root.Remote, usageCountConflict("month", root.Remote, total)
	}
	return root.Remote, nil
}

func (job *usageHistoryAccount) checkTotal(ctx context.Context, window usageHistoryWindow, walker *usageHistoryWalker) (usageWindowCounts, error) {
	item := UsageWindowIntegrity{AccountID: job.account.ID, AccountName: job.account.Username, Since: window.Since, Until: window.Until, Level: "total", Status: "checking"}
	if walker.Progress != nil {
		walker.Progress(item)
	}
	counts, err := job.count(ctx, window)
	if err != nil {
		return counts, err
	}
	walker.Report.CheckedWindows++
	item.Status = "mismatch"
	item.LocalCount = counts.Local
	item.UpstreamCount = counts.Remote
	if counts.Local == counts.Remote {
		item.Status = "matched"
		walker.Report.SkippedWindows++
	}
	if walker.Progress != nil {
		walker.Progress(item)
	}
	return counts, nil
}

func (s *Service) refreshChannelUsage(ctx context.Context, ch *storage.Channel) error {
	if ch.BalanceMode == storage.BalanceModeManual || s.channelUsage == nil {
		return nil
	}
	unlock, err := s.lockUsageChannel(ch.ID)
	if err != nil {
		return err
	}
	defer unlock()
	ctx = connector.WithUsageLimiter(ctx, s.usageLimiter)
	accounts, err := s.channels.ListAccounts(ch.ID)
	if err != nil {
		return err
	}
	var failures []error
	for _, account := range accounts {
		if err := ctx.Err(); err != nil {
			return err
		}
		job, err := s.prepareUsageAccount(ctx, ch, account)
		if err == nil {
			if ch.Type == storage.ChannelTypeNewAPI {
				err = job.refreshNewAPIUsage(ctx, time.Now().UTC())
				if err != nil {
					failures = append(failures, fmt.Errorf("%s usage: %w", account.Username, err))
				}
				continue
			}
			auditErr := job.auditAutomatically(ctx, time.Now().UTC())
			now := time.Now().UTC()
			since := channelUsageSyncStart(now, job.state, nil)
			if job.state.HistoryVerifiedUntil != nil && job.state.HistoryVerifiedUntil.After(since) {
				since = *job.state.HistoryVerifiedUntil
			}
			if job.state.HistoryResume != "" {
				var resume usageHistoryResume
				if json.Unmarshal([]byte(job.state.HistoryResume), &resume) == nil && resume.Until.After(since) {
					since = resume.Until
				}
			}
			err = job.readWindow(ctx, usageHistoryWindow{Since: since, Until: now}, -1, false)
			if err == nil {
				job.state.ForwardSyncedAt = &now
				err = s.channelUsage.SaveSyncState(job.state)
			}
			err = errors.Join(auditErr, err)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s usage: %w", account.Username, err))
		}
	}
	return errors.Join(failures...)
}

func (job *usageHistoryAccount) auditAutomatically(ctx context.Context, now time.Time) error {
	state := job.state
	if state.HistoryResume == "" && state.HistoryVerifiedUntil != nil && state.HistoryCheckedAt != nil && now.Sub(*state.HistoryCheckedAt) < 24*time.Hour {
		return nil
	}
	cutoff := now.Add(-2 * time.Minute).Truncate(5 * time.Minute)
	if state.HistoryResume == "" && state.HistoryVerifiedUntil != nil && !state.HistoryVerifiedUntil.Before(cutoff) {
		state.HistoryCheckedAt = &now
		return job.service.channelUsage.SaveSyncState(state)
	}
	return job.resumeHistory(ctx, cutoff, 3)
}
