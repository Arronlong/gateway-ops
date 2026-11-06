package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

type usageHistoryAccount struct {
	onHistoryPage func(int, int)
	checkedAt     time.Time
	service       *Service
	channel       *storage.Channel
	account       storage.ChannelAccount
	state         *storage.ChannelUsageSyncState
	resolved      *connector.Channel
	session       *connector.AuthSession
	reader        connector.UsageReader
	counter       connector.UsageCounter
}

func (s *Service) prepareUsageAccount(ctx context.Context, ch *storage.Channel, account storage.ChannelAccount) (*usageHistoryAccount, error) {
	state, err := s.channelUsage.SyncState(ch.ID, account.ID)
	if err != nil {
		return nil, err
	}
	return &usageHistoryAccount{service: s, channel: ch, account: account, state: state}, nil
}

func (job *usageHistoryAccount) ensurePrepared(ctx context.Context) error {
	if job.reader != nil && job.counter != nil {
		return nil
	}
	resolved, conn, session, err := job.service.prepare(ctx, job.service.channelSvc.AccountChannel(job.channel, job.account))
	if err != nil {
		return err
	}
	reader, ok := conn.(connector.UsageReader)
	if !ok {
		return fmt.Errorf("upstream does not expose usage history")
	}
	counter, ok := conn.(connector.UsageCounter)
	if !ok {
		return fmt.Errorf("upstream does not support reliable usage counts")
	}
	job.resolved = resolved
	job.session = session
	job.reader = reader
	job.counter = counter
	return nil
}

func (job *usageHistoryAccount) count(ctx context.Context, window usageHistoryWindow) (usageWindowCounts, error) {
	local, err := job.service.channelUsage.CountWindow(job.channel.ID, job.account.ID, string(job.channel.Type), window.Since, window.Until)
	if err != nil {
		return usageWindowCounts{}, err
	}
	retained, err := job.retainedUsageCount(window.Since, window.Until)
	if err != nil {
		return usageWindowCounts{}, err
	}
	local -= retained
	remote := job.service.cachedUsageCountWith(ctx, job.channel, job.account, window.Since, window.Until, func(readCtx context.Context) (int64, error) {
		if err := job.ensurePrepared(readCtx); err != nil {
			return 0, err
		}
		return job.counter.CountUsage(readCtx, job.resolved, job.session, connector.UsageQuery{Since: window.Since, Until: window.Until})
	})
	if job.checkedAt.IsZero() || remote.checkedAt.Before(job.checkedAt) {
		job.checkedAt = remote.checkedAt
	}
	return usageWindowCounts{Local: local, Remote: remote.count}, remote.err
}

func (job *usageHistoryAccount) walker(ctx context.Context, report *UsageHistoryReport, repair bool) *usageHistoryWalker {
	walker := &usageHistoryWalker{AccountID: job.account.ID, AccountName: job.account.Username, Report: report, Count: job.count, SplitDense: repair && job.channel.Type == storage.ChannelTypeNewAPI, Incremental: repair}
	if repair {
		walker.Repair = func(ctx context.Context, window usageHistoryWindow, expected int64) error {
			return job.readWindow(ctx, window, expected, true)
		}
	}
	walker.Progress = func(item UsageWindowIntegrity) {
		progress.FromContext(ctx).Emit(progress.Event{Stage: progress.StageUsage, Message: usageHistoryWindowMessage(item), Data: map[string]any{"phase": item.Level, "status": item.Status, "checked_windows": report.CheckedWindows, "skipped_windows": report.SkippedWindows, "repaired_days": report.RepairedDays}, Time: time.Now()})
	}
	return walker
}

func usageHistoryWindowMessage(item UsageWindowIntegrity) string {
	level := map[string]string{"total": "\u603b\u6570", "month": "\u6708", "week": "\u5468", "day": "\u5929", "hour": "\u5c0f\u65f6", "remaining": "\u5269\u4f59\u533a\u95f4", "history": "\u5386\u53f2\u8303\u56f4"}[item.Level]
	outcome := "\u53d1\u73b0\u6761\u6570\u5dee\u5f02\uff0c\u7ee7\u7eed\u5b9a\u4f4d"
	if item.Status == "checking" {
		outcome = "\u6b63\u5728\u6838\u5bf9"
	}
	if item.Status == "matched" {
		outcome = "\u6761\u6570\u4e00\u81f4\uff0c\u8df3\u8fc7"
	}
	if item.Status == "repaired" {
		outcome = "\u5f02\u5e38\u533a\u95f4\u4fee\u590d\u5b8c\u6210"
	}
	if item.Status == "subdividing" {
		outcome = "\u4e0a\u6e38\u603b\u6570\u53ef\u80fd\u622a\u65ad\u6216\u533a\u95f4\u8fc7\u5927\uff0c\u7ee7\u7eed\u7f29\u5c0f\u8303\u56f4"
	}
	if item.Status == "repairing" {
		outcome = "\u5df2\u4fdd\u5b58\u5f53\u524d\u9875\uff0c\u7ee7\u7eed\u8bfb\u53d6"
	}
	if item.Status == "verified" {
		outcome = "\u5386\u53f2\u8303\u56f4\u6838\u5bf9\u5b8c\u6210"
	}
	if item.Status == "interrupted" {
		outcome = "\u5904\u7406\u672a\u5b8c\u6210\uff0c\u5df2\u4fdd\u7559\u6536\u5230\u7684\u6570\u636e\u548c\u65ad\u70b9"
	}
	since := item.Since.Format(time.RFC3339)
	if item.Since.IsZero() {
		since = "\u5168\u90e8\u5386\u53f2"
	}
	message := fmt.Sprintf("%s: %s\u6821\u9a8c %s - %s\uff1a%s", item.AccountName, level, since, item.Until.Format(time.RFC3339), outcome)
	if item.Status != "checking" {
		message += fmt.Sprintf("\uff08\u672c\u5730 %d / \u4e0a\u6e38 %d\uff09", item.LocalCount, item.UpstreamCount)
	}
	return message
}

func (job *usageHistoryAccount) readWindow(ctx context.Context, window usageHistoryWindow, expected int64, replace bool) error {
	if job.reader == nil {
		if err := job.ensurePrepared(ctx); err != nil {
			return err
		}
	}
	if expected > 100000 {
		return fmt.Errorf("usage day exceeds the bounded repair limit; retained existing history")
	}
	const pageSize = 100
	now := time.Now().UTC()
	var events []storage.ChannelUsageEvent
	occurrences := make(map[string]int)
	var previousStamp time.Time
	for page := 1; page <= 1000; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		progress.Start(ctx, progress.StageUsage, fmt.Sprintf("%s: %s - %s \u6b63\u5728\u8bfb\u53d6\u7b2c %d \u9875", job.account.Username, window.Since.Format(time.RFC3339), window.Until.Format(time.RFC3339), page))
		result, err := job.reader.ListUsage(ctx, job.resolved, job.session, connector.UsageQuery{Since: window.Since, Until: window.Until, Page: page, PageSize: pageSize})
		if err != nil {
			return err
		}
		if result == nil {
			return fmt.Errorf("upstream returned an empty page response")
		}
		crossedStart := false
		pageStart := len(events)
		if job.channel.Type == storage.ChannelTypeSub2API && result.RawCount != len(result.Records) {
			return fmt.Errorf("upstream returned usage rows without valid timestamps")
		}
		for _, record := range result.Records {
			if job.channel.Type == storage.ChannelTypeSub2API {
				if !previousStamp.IsZero() && record.CreatedAt.After(previousStamp) {
					return fmt.Errorf("upstream ignored descending usage order; retained existing history")
				}
				previousStamp = record.CreatedAt
				if record.CreatedAt.Before(window.Since) {
					crossedStart = true
				}
			}
			if record.RemoteID == "" || record.CreatedAt.IsZero() || record.CreatedAt.Before(window.Since) || !record.CreatedAt.Before(window.Until) {
				continue
			}
			remoteID := record.RemoteID
			if job.channel.Type == storage.ChannelTypeNewAPI {
				occurrences[remoteID]++
				if occurrences[remoteID] > 1 {
					remoteID = fmt.Sprintf("%s:%d", remoteID, occurrences[remoteID])
				}
			}
			events = append(events, storage.ChannelUsageEvent{CacheKnown: record.CacheKnown, ChannelID: job.channel.ID, ChannelAccountID: job.account.ID, Source: string(job.channel.Type), RemoteUsageID: remoteID, RequestID: record.RequestID, Model: record.Model, FirstTokenMS: record.FirstTokenMS, DurationMS: record.DurationMS, SuccessKnown: record.SuccessKnown, Success: record.Success, InputTokens: record.InputTokens, OutputTokens: record.OutputTokens, CacheReadTokens: record.CacheReadTokens, CacheCreationTokens: record.CacheCreationTokens, ActualCost: record.ActualCost, TotalCost: record.TotalCost, OccurredAt: record.CreatedAt.UTC(), SyncedAt: now})
		}
		if err := job.service.channelUsage.Upsert(events[pageStart:]); err != nil {
			return err
		}
		if expected >= 0 && int64(len(events)) > expected {
			return fmt.Errorf("upstream returned more calls than its verified total; saved received pages without replacing the incomplete window")
		}
		if crossedStart || usagePageComplete(result, pageSize) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if expected >= 0 && int64(len(events)) != expected {
				return fmt.Errorf("incomplete usage history: fetched %d of %d calls; retained existing history", len(events), expected)
			}
			if replace {
				return job.service.channelUsage.ReplaceHistory(job.state, string(job.channel.Type), window.Since, window.Until, events)
			}
			return nil
		}
	}
	return fmt.Errorf("usage pagination exceeded 1000 pages; retained existing history")
}
