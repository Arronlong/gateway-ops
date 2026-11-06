package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func (s *Service) refetchNewAPIHistory(ctx context.Context, ch *storage.Channel, since, until time.Time) (*UsageHistoryReport, error) {
	report := &UsageHistoryReport{}
	result := &UsageIntegrity{Status: "unavailable", Phase: "locating", Since: since, Until: until, CheckedAt: time.Now().UTC(), Audit: report, Accounts: []UsageAccountIntegrity{}}
	report.Result = result
	accounts, err := s.channels.ListAccounts(ch.ID)
	if err != nil {
		return report, err
	}
	if len(accounts) == 0 {
		return report, fmt.Errorf("channel has no upstream accounts")
	}
	for _, account := range accounts {
		result.Accounts = append(result.Accounts, UsageAccountIntegrity{AccountID: account.ID, AccountName: account.Username})
	}
	emitUsageIntegrity(ctx, result, "locating", "\u6b63\u5728\u6309\u6708\u6838\u5bf9\uff0c\u622a\u65ad\u7684\u603b\u6570\u4e0d\u4f5c\u4e3a\u51c6\u786e\u6761\u6570")
	var failures []error
	for i, account := range accounts {
		job, readErr := s.prepareUsageAccount(ctx, ch, account)
		if readErr == nil {
			job.onHistoryPage = func(page, saved int) {
				_ = job.refreshAvailableResult(result, i)
				emitUsageIntegrity(ctx, result, "repairing", fmt.Sprintf("%s: \u7b2c %d \u9875\u5df2\u4fdd\u5b58\uff0c\u7d2f\u8ba1 %d \u9875\u3001%d \u6761\u8c03\u7528", account.Username, page, report.SavedPages, report.SavedRows))
			}
			readErr = job.scanNewAPIHistory(ctx, since, until, 1000, report)
			if refreshErr := job.refreshAvailableResult(result, i); readErr == nil {
				readErr = refreshErr
			}
			if readErr == nil {
				scan, stateErr := job.newAPIScanState()
				if stateErr != nil {
					readErr = stateErr
				} else {
					count := scan.CompletedCalls
					result.Accounts[i].UpstreamCount = &count
					result.Accounts[i].RetainedCount = scan.CompletedRetained
				}
			}
		}
		if readErr != nil {
			result.Accounts[i].Error = readErr.Error()
			failures = append(failures, fmt.Errorf("%s: %w", account.Username, readErr))
			break
		}
	}
	result.LocalCount, _ = s.channelUsage.CountWindow(ch.ID, 0, string(ch.Type), since, until)
	refreshUsageIntegrityTotals(result)
	err = errors.Join(failures...)
	if err == nil && result.Status == "mismatch" {
		err = fmt.Errorf("\u5206\u6bb5\u6838\u5bf9\u540e\u4ecd\u6709\u5dee\u5f02\uff0c\u5df2\u4fdd\u5b58\u6536\u5230\u7684\u6570\u636e")
	}
	if err != nil {
		result.Error = err.Error()
		s.emitUsageFailure(ctx, ch, result, "\u5df2\u4fdd\u5b58\u6570\u636e\u548c\u5206\u6bb5\u8fdb\u5ea6\uff0c\u4e0b\u6b21\u4ece\u672a\u5b8c\u6210\u4f4d\u7f6e\u7ee7\u7eed")
	} else {
		emitUsageIntegrity(ctx, result, "complete", "\u5386\u53f2\u6570\u636e\u5df2\u5206\u6bb5\u6838\u5bf9\u5e76\u9010\u9875\u4fdd\u5b58")
	}
	return report, err
}

func (job *usageHistoryAccount) refreshNewAPIUsage(ctx context.Context, now time.Time) error {
	scan, err := job.newAPIScanState()
	if err != nil {
		return err
	}
	var auditErr error
	if scan.Plans["full"] != nil || job.state.HistoryScan == "" || job.state.HistoryCheckedAt == nil || now.Sub(*job.state.HistoryCheckedAt) >= 24*time.Hour {
		cutoff := now.Add(-2 * time.Minute).Truncate(5 * time.Minute)
		auditErr = job.scanNewAPIHistory(ctx, time.Time{}, cutoff, 3, &UsageHistoryReport{})
		if errors.Is(auditErr, errUsageScanBudget) {
			auditErr = nil
		}
	}
	since := channelUsageSyncStart(now, job.state, nil)
	readErr := job.scanNewAPIHistoryScope(ctx, since, now, 50, &UsageHistoryReport{}, "recent")
	if readErr == nil {
		job.state.ForwardSyncedAt = &now
		readErr = job.service.channelUsage.SaveSyncState(job.state)
	}
	return errors.Join(auditErr, readErr)
}
