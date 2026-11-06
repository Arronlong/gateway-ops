package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
	"github.com/guliansheng/gateway-ops/internal/progress"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

// 历史连续完成位置独立于近期同步，最新页不能跳过历史缺口。
type usageHistoryResume struct {
	ID                        string
	Since, Until              time.Time
	Source                    string
	NextPage, Pages, PageSize int
	Total, RawRows            int64
	Boundary, Oldest          time.Time
	FirstOldest               time.Time
}

func (job *usageHistoryAccount) resumeHistory(ctx context.Context, until time.Time, pageBudget int) (retErr error) {
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
	var run usageHistoryResume
	if job.state.HistoryResume != "" {
		if err := json.Unmarshal([]byte(job.state.HistoryResume), &run); err != nil {
			return fmt.Errorf("invalid usage history checkpoint: %w", err)
		}
	}
	for used := 0; used < pageBudget; used++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if run.ID == "" {
			since := time.Time{}
			if job.state.HistoryVerifiedUntil != nil {
				since = *job.state.HistoryVerifiedUntil
			}
			if !since.Before(until) {
				return nil
			}
			run = usageHistoryResume{ID: fmt.Sprintf("%d-%d-%d", job.channel.ID, job.account.ID, time.Now().UnixNano()), Since: since, Until: until, Source: string(job.channel.Type), PageSize: 100}
		}
		if run.Source != string(job.channel.Type) {
			return fmt.Errorf("usage source changed during historical verification")
		}
		page := run.NextPage
		if page == 0 {
			page = 1
		}
		progress.Start(ctx, progress.StageUsage, fmt.Sprintf("%s: \u8bfb\u53d6\u7b2c %d \u9875\uff0c\u53d6\u5f97\u540e\u7acb\u5373\u6838\u5bf9\u5e76\u4fdd\u5b58", job.account.Username, page))
		result, err := job.reader.ListUsage(ctx, job.resolved, job.session, connector.UsageQuery{Since: run.Since, Until: run.Until, Page: page, PageSize: run.PageSize})
		if err != nil {
			return err
		}
		if result == nil {
			return fmt.Errorf("upstream returned an empty usage page response")
		}
		events := job.historyPageEvents(result.Records, run.Since, run.Until)
		identities := make([]string, 0, len(events))
		for _, record := range result.Records {
			if record.RemoteID == "" || record.CreatedAt.IsZero() || record.CreatedAt.Before(run.Since) || !record.CreatedAt.Before(run.Until) {
				continue
			}
			identity := record.PageIdentity
			if identity == "" {
				identity = record.RemoteID
			}
			identities = append(identities, identity)
		}
		validErr := validateHistoryPage(result, page, &run)
		if validErr == nil {
			overlaps, checkErr := job.service.channelUsage.HistoryPageOverlaps(run.ID, identities)
			if checkErr != nil {
				return checkErr
			}
			if overlaps {
				validErr = fmt.Errorf("upstream usage pages overlap or contain ambiguous duplicate rows; saved data without advancing the continuous boundary")
			}
		}
		if validErr != nil {
			if err := job.service.channelUsage.CommitHistoryPage(job.state, run.ID, run.Source, events, identities, time.Time{}, time.Time{}, false); err != nil {
				return err
			}
			if err := job.service.channelUsage.DiscardHistoryRun(job.state, run.ID); err != nil {
				return err
			}
			if job.onHistoryPage != nil {
				job.onHistoryPage(page, len(events))
			}
			return validErr
		}
		// 首页立即保存，再从末页向前核对，不重复读取首页。
		first := run.NextPage == 0
		newest, oldest := time.Time{}, time.Time{}
		if len(result.RawTimes) > 0 {
			newest = result.RawTimes[0]
			oldest = result.RawTimes[len(result.RawTimes)-1]
		}
		if first {
			run.Total = result.Total
			run.PageSize = result.PageSize
			run.Pages = result.Pages
			if run.Pages < 1 {
				run.Pages = 1
			}
			run.FirstOldest = oldest
			run.NextPage = run.Pages
		} else {
			if (!run.Boundary.IsZero() && oldest.Before(run.Boundary)) || (page == 2 && !run.FirstOldest.IsZero() && newest.After(run.FirstOldest)) {
				if err := job.service.channelUsage.CommitHistoryPage(job.state, run.ID, run.Source, events, identities, time.Time{}, time.Time{}, false); err != nil {
					return err
				}
				if err := job.service.channelUsage.DiscardHistoryRun(job.state, run.ID); err != nil {
					return err
				}
				return fmt.Errorf("upstream usage order changed; saved received rows without advancing the history boundary")
			}
			run.NextPage--
		}
		run.RawRows += int64(result.RawCount)
		if !first && run.Oldest.IsZero() {
			run.Oldest = oldest
		}
		done := run.Pages == 1 || (!first && page == 2)
		if done && run.RawRows != run.Total {
			return fmt.Errorf("incomplete usage page coverage: %d of %d", run.RawRows, run.Total)
		}
		cleanSince, cleanUntil := time.Time{}, time.Time{}
		if !first || done {
			cleanSince = run.Since
			if cleanSince.IsZero() {
				cleanSince = run.Oldest
				if done && cleanSince.IsZero() {
					cleanSince = oldest
				}
			}
			if job.state.HistoryVerifiedUntil != nil && job.state.HistoryVerifiedUntil.After(cleanSince) {
				cleanSince = *job.state.HistoryVerifiedUntil
			}
			// 同一时间戳可能跨页，连续边界不包含该时刻。
			cleanUntil = newest
			if done {
				cleanUntil = run.Until
			}
			if cleanUntil.After(run.Until) {
				cleanUntil = run.Until
			}
			if cleanSince.IsZero() {
				cleanUntil = time.Time{}
			}
			if !cleanUntil.IsZero() && (job.state.HistoryVerifiedUntil == nil || cleanUntil.After(*job.state.HistoryVerifiedUntil)) {
				boundary := cleanUntil
				job.state.HistoryVerifiedUntil = &boundary
			}
			run.Boundary = newest
		}
		encoded, err := json.Marshal(run)
		if err != nil {
			return err
		}
		job.state.HistoryResume = string(encoded)
		if done {
			job.state.HistoryResume = ""
			if run.Since.IsZero() {
				available := run.Oldest
				if available.IsZero() {
					available = oldest
				}
				if available.IsZero() {
					available = run.Until
				}
				job.state.HistoryAvailableSince = &available
			}
			boundary := run.Until
			job.state.HistoryVerifiedUntil = &boundary
			now := time.Now().UTC()
			job.state.HistoryCheckedAt = &now
			job.state.BackfillVersion = channelUsageBackfillVersion
			job.state.HistoryAuditBefore = nil
			job.state.HistoryAuditRemaining = nil
		}
		if err := job.service.channelUsage.CommitHistoryPage(job.state, run.ID, run.Source, events, identities, cleanSince, cleanUntil, done); err != nil {
			return err
		}
		if job.onHistoryPage != nil {
			job.onHistoryPage(page, len(events))
		}
		progress.FromContext(ctx).Emit(progress.Event{Stage: progress.StageUsage, Message: fmt.Sprintf("%s: \u7b2c %d \u9875\u5df2\u6838\u5bf9\u5e76\u4fdd\u5b58 %d \u6761\u8c03\u7528", job.account.Username, page, len(events)), Data: map[string]any{"kind": "usage_page_saved", "page": page, "pages": run.Pages, "saved_rows": len(events), "history_verified_until": job.state.HistoryVerifiedUntil}, Time: time.Now()})
		if done {
			if !run.Until.Before(until) {
				return nil
			}
			run = usageHistoryResume{}
		}
	}
	return nil
}

func validateHistoryPage(page *connector.UsagePage, requested int, run *usageHistoryResume) error {
	if !page.TotalKnown || page.Total < 0 || page.PageSize <= 0 || page.Page != requested {
		return fmt.Errorf("upstream did not provide reliable usage pagination metadata")
	}
	pages := int((page.Total + int64(page.PageSize) - 1) / int64(page.PageSize))
	if pages < 1 {
		pages = 1
	}
	if page.Pages > 0 && page.Pages != pages {
		return fmt.Errorf("upstream usage page count is inconsistent")
	}
	page.Pages = pages
	if run.NextPage != 0 && (page.Total != run.Total || page.PageSize != run.PageSize || pages != run.Pages) {
		return fmt.Errorf("upstream usage pagination changed; saved received rows and will resume from the continuous boundary")
	}
	expected := page.PageSize
	if requested == pages {
		expected = int(page.Total - int64((pages-1)*page.PageSize))
	}
	if page.RawCount != expected || len(page.RawTimes) != page.RawCount {
		return fmt.Errorf("upstream usage page is incomplete or contains invalid timestamps")
	}
	for i, stamp := range page.RawTimes {
		page.RawTimes[i] = stamp.UTC()
		if stamp.IsZero() || (i > 0 && stamp.After(page.RawTimes[i-1])) {
			return fmt.Errorf("upstream usage is not ordered from newest to oldest")
		}
		start, end := run.Since, run.Until
		if run.Source == string(storage.ChannelTypeSub2API) {
			if !start.IsZero() {
				start = start.UTC().Truncate(24 * time.Hour)
			}
			end = end.UTC().Add(-time.Nanosecond).Truncate(24 * time.Hour).Add(24 * time.Hour)
		}
		if (!start.IsZero() && stamp.Before(start)) || !stamp.Before(end) {
			return fmt.Errorf("upstream returned usage outside the requested time window; continuous boundary was not advanced")
		}
	}
	return nil
}

func (job *usageHistoryAccount) historyPageEvents(records []connector.UsageRecord, since, until time.Time) []storage.ChannelUsageEvent {
	events := make([]storage.ChannelUsageEvent, 0, len(records))
	for _, record := range records {
		if record.RemoteID == "" || record.CreatedAt.IsZero() || record.CreatedAt.Before(since) || !record.CreatedAt.Before(until) {
			continue
		}
		events = append(events, storage.ChannelUsageEvent{CacheKnown: record.CacheKnown, ChannelID: job.channel.ID, ChannelAccountID: job.account.ID, Source: string(job.channel.Type), RemoteUsageID: record.RemoteID, RequestID: record.RequestID, Model: record.Model, FirstTokenMS: record.FirstTokenMS, DurationMS: record.DurationMS, SuccessKnown: record.SuccessKnown, Success: record.Success, InputTokens: record.InputTokens, OutputTokens: record.OutputTokens, CacheReadTokens: record.CacheReadTokens, CacheCreationTokens: record.CacheCreationTokens, ActualCost: record.ActualCost, TotalCost: record.TotalCost, OccurredAt: record.CreatedAt.UTC(), SyncedAt: time.Now().UTC()})
	}
	return events
}
