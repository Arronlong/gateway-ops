package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func (s *Service) refetchContinuousHistory(ctx context.Context, ch *storage.Channel, until time.Time) (*UsageHistoryReport, error) {
	result, _, err := s.checkUsageIntegrity(ctx, ch, time.Time{}, until, false, true)
	if result.Audit == nil {
		result.Audit = &UsageHistoryReport{}
	}
	report := result.Audit
	report.Result = result
	if err != nil {
		return report, err
	}
	accounts, err := s.channels.ListAccounts(ch.ID)
	if err != nil {
		return report, err
	}
	if len(accounts) == 0 {
		return report, fmt.Errorf("channel has no upstream accounts")
	}
	var failures []error
	for i, account := range accounts {
		job, readErr := s.prepareUsageAccount(ctx, ch, account)
		if readErr == nil {
			hadCoverage := job.state.HistoryAvailableSince != nil
			job.onHistoryPage = func(page, saved int) {
				report.SavedPages++
				report.SavedRows += saved
				_ = job.refreshAvailableResult(result, i)
				emitUsageIntegrity(ctx, result, "repairing", fmt.Sprintf("%s: \u7b2c %d \u9875\u5df2\u6838\u5bf9\u5e76\u4fdd\u5b58\uff0c\u672c\u6b21\u5df2\u4fdd\u5b58 %d \u9875\u3001%d \u6761\u8c03\u7528", account.Username, page, report.SavedPages, report.SavedRows))
			}
			readErr = job.resumeHistory(ctx, until, 1000)
			if readErr == nil && job.state.HistoryResume != "" {
				readErr = fmt.Errorf("historical usage page budget reached; saved the continuous checkpoint for the next run")
			}
			if readErr == nil {
				readErr = job.reconcileAvailableHistory(ctx, result, i, hadCoverage)
			}
			result.Accounts[i].HistoryVerifiedUntil = job.state.HistoryVerifiedUntil
		}
		if readErr != nil {
			result.Accounts[i].Error = readErr.Error()
			failures = append(failures, fmt.Errorf("%s: %w", account.Username, readErr))
			break
		}
		result.Accounts[i].Error = ""
	}
	result.LocalCount, _ = s.channelUsage.CountWindow(ch.ID, 0, string(ch.Type), time.Time{}, until)
	refreshUsageIntegrityTotals(result)
	err = errors.Join(failures...)
	if err == nil && result.Status == "mismatch" {
		upstream := "\u672a\u53d6\u5f97"
		if result.UpstreamCount != nil {
			upstream = fmt.Sprint(*result.UpstreamCount)
		}
		err = fmt.Errorf("\u6838\u5bf9\u540e\u4ecd\u6709\u5dee\u5f02\uff08\u672c\u5730\u53ef\u6838\u5bf9 %d \u6761\uff0c\u4e0a\u6e38 %s\uff09\uff0c\u8bf7\u67e5\u770b\u8d26\u53f7\u5dee\u5f02", result.LocalCount-result.RetainedCount, upstream)
	}
	if err != nil {
		result.Error = err.Error()
		s.emitUsageFailure(ctx, ch, result, "\u5df2\u4fdd\u5b58\u6536\u5230\u7684\u6bcf\u9875\u6570\u636e\uff0c\u4e0b\u6b21\u4ece\u8fde\u7eed\u6838\u5bf9\u5b8c\u6210\u4f4d\u7f6e\u7ee7\u7eed")
	} else {
		emitUsageIntegrity(ctx, result, "complete", "\u5386\u53f2\u6570\u636e\u5df2\u9010\u9875\u6838\u5bf9\u5e76\u4fdd\u5b58")
	}
	return report, err
}
