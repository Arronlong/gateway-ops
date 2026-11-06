package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

type usageHistoryRepair struct {
	job    *usageHistoryAccount
	window usageHistoryWindow
	counts usageWindowCounts
}

func refreshUsageIntegrityTotals(result *UsageIntegrity) {
	var total int64
	result.RetainedCount = 0
	known, mismatch := len(result.Accounts) > 0, false
	for _, account := range result.Accounts {
		result.RetainedCount += account.RetainedCount
		if account.UpstreamCount == nil {
			known = false
			continue
		}
		total += *account.UpstreamCount
		mismatch = mismatch || account.LocalCount-account.RetainedCount != *account.UpstreamCount
	}
	result.UpstreamCount = nil
	result.Status = "unavailable"
	if known {
		result.UpstreamCount = &total
		result.Status = "matched"
		mismatch = mismatch || total != result.LocalCount-result.RetainedCount
	}
	if mismatch {
		result.Status = "mismatch"
	}
}

func usageIntegrityFailure(result *UsageIntegrity) error {
	var failures []error
	for _, account := range result.Accounts {
		if account.Error != "" {
			failures = append(failures, fmt.Errorf("%s: %s", account.AccountName, account.Error))
		}
	}
	return errors.Join(failures...)
}

func emitUsageIntegrity(ctx context.Context, result *UsageIntegrity, phase, message string) {
	result.Phase = phase
	snapshot := *result
	snapshot.Accounts = append([]UsageAccountIntegrity(nil), result.Accounts...)
	if result.Audit != nil {
		audit := *result.Audit
		audit.Result = nil
		audit.Windows = append([]UsageWindowIntegrity(nil), result.Audit.Windows...)
		snapshot.Audit = &audit
	}
	progress.FromContext(ctx).Emit(progress.Event{Stage: progress.StageUsage, Message: message, Data: map[string]any{"kind": "usage_integrity", "phase": phase, "result": snapshot}, Time: time.Now()})
}

func (s *Service) checkUsageIntegrity(ctx context.Context, ch *storage.Channel, since, until time.Time, planRepair bool, totalsOnly ...bool) (*UsageIntegrity, []usageHistoryRepair, error) {
	ctx = connector.WithUsageLimiter(ctx, s.usageLimiter)
	result := &UsageIntegrity{Status: "unavailable", Phase: "total", Since: since, Until: until, CheckedAt: time.Now().UTC(), Accounts: []UsageAccountIntegrity{}}
	if ch.BalanceMode == storage.BalanceModeManual {
		result.Status = "not_applicable"
		result.Phase = "complete"
		return result, nil, nil
	}
	if s.channelUsage == nil || !since.Before(until) {
		return result, nil, nil
	}
	accounts, err := s.channels.ListAccounts(ch.ID)
	if err != nil {
		return result, nil, err
	}
	result.LocalCount, err = s.channelUsage.CountWindow(ch.ID, 0, string(ch.Type), since, until)
	if err != nil {
		return result, nil, err
	}
	if len(accounts) == 0 {
		return result, nil, nil
	}
	result.Audit = &UsageHistoryReport{}
	for _, account := range accounts {
		local, err := s.channelUsage.CountWindow(ch.ID, account.ID, string(ch.Type), since, until)
		if err != nil {
			return result, nil, err
		}
		result.Accounts = append(result.Accounts, UsageAccountIntegrity{AccountID: account.ID, AccountName: account.Username, LocalCount: local})
	}
	type target struct {
		job    *usageHistoryAccount
		counts usageWindowCounts
		index  int
	}
	var targets []target
	window := usageHistoryWindow{Since: since, Until: until}
	for index, account := range accounts {
		if ctx.Err() != nil {
			return result, nil, ctx.Err()
		}
		job, readErr := s.prepareUsageAccount(ctx, ch, account)
		if job != nil {
			result.Accounts[index].HistoryVerifiedUntil = job.state.HistoryVerifiedUntil
			result.Accounts[index].HistoryAvailableSince = job.state.HistoryAvailableSince
			retained, countErr := job.retainedUsageCount(since, until)
			if countErr != nil {
				return result, nil, countErr
			}
			result.Accounts[index].RetainedCount = retained
		}
		var counts usageWindowCounts
		if readErr == nil {
			counts, readErr = job.checkTotal(ctx, window, job.walker(ctx, result.Audit, false))
			if !job.checkedAt.IsZero() && job.checkedAt.Before(result.CheckedAt) {
				result.CheckedAt = job.checkedAt
			}
		}
		if readErr != nil {
			result.Accounts[index].Error = readErr.Error()
		} else {
			remote := counts.Remote
			result.Accounts[index].UpstreamCount = &remote
			targets = append(targets, target{job: job, counts: counts, index: index})
		}
		refreshUsageIntegrityTotals(result)
		emitUsageIntegrity(ctx, result, "total", fmt.Sprintf("\u603b\u4f53\u6761\u6570\u5df2\u6838\u5bf9 %d/%d \u4e2a\u8d26\u53f7", index+1, len(accounts)))
	}
	var repairs []usageHistoryRepair
	if len(totalsOnly) > 0 && totalsOnly[0] {
		if failure := usageIntegrityFailure(result); failure != nil {
			result.Error = failure.Error()
			s.emitUsageFailure(ctx, ch, result, result.Error)
		} else {
			emitUsageIntegrity(ctx, result, "complete", "\u603b\u4f53\u6761\u6570\u6838\u5bf9\u5b8c\u6210\uff0c\u672a\u626b\u63cf\u5386\u53f2\u5206\u6bb5")
		}
		return result, nil, nil
	}
	for _, target := range targets {
		if target.counts.Local == target.counts.Remote {
			continue
		}
		job := target.job
		walker := job.walker(ctx, result.Audit, false)
		walker.SplitDense = planRepair && ch.Type == storage.ChannelTypeNewAPI
		if planRepair {
			walker.Incremental = true
			walker.Repair = func(ctx context.Context, w usageHistoryWindow, c int64) error {
				local, err := s.channelUsage.CountWindow(ch.ID, job.account.ID, string(ch.Type), w.Since, w.Until)
				if err != nil {
					return err
				}
				repair := usageHistoryRepair{job: job, window: w, counts: usageWindowCounts{Local: local, Remote: c}}
				if err := s.executeUsageHistoryRepair(ctx, result, repair); err != nil {
					return err
				}
				repairs = append(repairs, repair)
				return nil
			}
		}
		walker.Progress = func(item UsageWindowIntegrity) {
			phase := "locating"
			if item.Status == "repaired" {
				phase = "repairing"
			}
			emitUsageIntegrity(ctx, result, phase, usageHistoryWindowMessage(item))
		}
		_, readErr := job.auditKnownRange(ctx, window, walker, target.counts)
		if planRepair && errors.Is(readErr, errUsageCountConflict) {
			s.usageAudit.invalidate(ch.ID)
			emitUsageIntegrity(ctx, result, "locating", "\u5206\u6bb5\u7edf\u8ba1\u4e0e\u603b\u6570\u4e0d\u4e00\u81f4\uff0c\u6b63\u5728\u91cd\u65b0\u83b7\u53d6\u672c\u8f6e\u7edf\u8ba1")
			fresh, err := job.count(ctx, window)
			readErr = err
			if err == nil {
				account := &result.Accounts[target.index]
				result.LocalCount += fresh.Local + account.RetainedCount - account.LocalCount
				account.LocalCount = fresh.Local + account.RetainedCount
				remote := fresh.Remote
				account.UpstreamCount = &remote
				refreshUsageIntegrityTotals(result)
				_, readErr = job.auditKnownRange(ctx, window, walker, fresh)
			}
		}
		if readErr != nil {
			result.Accounts[target.index].Error = readErr.Error()
		}
	}
	if failure := usageIntegrityFailure(result); failure != nil {
		result.Error = failure.Error()
		s.emitUsageFailure(ctx, ch, result, "\u5df2\u4fdd\u7559\u5df2\u5b8c\u6210\u7684\u4fee\u590d\uff0c\u5176\u4f59\u533a\u95f4\u5c1a\u672a\u5b8c\u6210")
	} else {
		phase := "complete"
		emitUsageIntegrity(ctx, result, phase, "\u5f02\u5e38\u533a\u95f4\u5b9a\u4f4d\u5b8c\u6210")
	}
	return result, repairs, nil
}

func (s *Service) executeUsageHistoryRepair(ctx context.Context, result *UsageIntegrity, repair usageHistoryRepair) error {
	if repair.counts.Remote == 0 {
		return fmt.Errorf("%s: \u4e0a\u6e38 %s \u5df2\u65e0\u65e5\u5fd7\uff0c\u672c\u5730 %d \u6761\u5386\u53f2\u5df2\u4fdd\u7559\uff0c\u65e0\u6cd5\u4ece\u4e0a\u6e38\u8865\u56de", repair.job.account.Username, repair.window.Since.Format("2006-01-02"), repair.counts.Local)
	}
	message := fmt.Sprintf("\u6b63\u5728\u4fee\u590d\u5df2\u786e\u8ba4\u7684\u5f02\u5e38\u533a\u95f4\uff1a%s %s - %s", repair.job.account.Username, repair.window.Since.Format(time.RFC3339), repair.window.Until.Format(time.RFC3339))
	emitUsageIntegrity(ctx, result, "repairing", message)
	if err := repair.job.readWindow(ctx, repair.window, repair.counts.Remote, true); err != nil {
		return err
	}
	for index := range result.Accounts {
		if result.Accounts[index].AccountID != repair.job.account.ID {
			continue
		}
		local, err := s.channelUsage.CountWindow(repair.job.channel.ID, repair.job.account.ID, string(repair.job.channel.Type), result.Since, result.Until)
		if err != nil {
			return err
		}
		result.LocalCount += local - result.Accounts[index].LocalCount
		result.Accounts[index].LocalCount = local
	}
	refreshUsageIntegrityTotals(result)
	return nil
}
