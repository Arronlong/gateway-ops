package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
	"github.com/guliansheng/gateway-ops/internal/tasklog"
)

const channelUsageIntegrityTimezone = "Asia/Shanghai"

func UsageHistoryBounds(preset string, now time.Time, location *time.Location) (time.Time, time.Time, error) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	if location == nil {
		var err error
		location, err = time.LoadLocation(channelUsageIntegrityTimezone)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("load usage timezone: %w", err)
		}
	}
	if now.IsZero() {
		now = time.Now().In(location)
	}
	switch preset {
	case "today":
		localNow := now.In(location)
		return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location), now, nil
	case "24h":
		return now.Add(-24 * time.Hour), now, nil
	case "7d":
		return now.Add(-7 * 24 * time.Hour), now, nil
	case "all":
		return time.Time{}, now.UTC().Add(-2 * time.Minute).Truncate(5 * time.Minute), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported usage integrity range %q", preset)
	}
}

type UsageHistoryAllChannelError struct {
	ChannelID   uint
	ChannelName string
	Err         error
}

type UsageHistoryAllSkippedChannel struct {
	ChannelID   uint
	ChannelName string
}

type UsageHistoryAllResult struct {
	Total           int
	Succeeded       int
	Failed          int
	Skipped         int
	SkippedChannels []UsageHistoryAllSkippedChannel
	Errors          []UsageHistoryAllChannelError
}

func runAllUsageHistory(ctx context.Context, channels []storage.Channel, since, until time.Time, refetch func(context.Context, *storage.Channel, time.Time, time.Time) error) UsageHistoryAllResult {
	result := UsageHistoryAllResult{Total: len(channels)}
	for index := range channels {
		if err := ctx.Err(); err != nil {
			result.Failed += len(channels) - index
			result.Errors = append(result.Errors, UsageHistoryAllChannelError{Err: err})
			return result
		}
		channel := &channels[index]
		if channel.BalanceMode == storage.BalanceModeManual {
			result.Skipped++
			result.SkippedChannels = append(result.SkippedChannels, UsageHistoryAllSkippedChannel{ChannelID: channel.ID, ChannelName: channel.Name})
			continue
		}
		if err := refetch(ctx, channel, since, until); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, UsageHistoryAllChannelError{ChannelID: channel.ID, ChannelName: channel.Name, Err: err})
			continue
		}
		result.Succeeded++
	}
	return result
}

func (s *Service) RefetchAllUsageHistory(ctx context.Context, since, until time.Time) (UsageHistoryAllResult, error) {
	channels, err := s.channels.List()
	if err != nil {
		return UsageHistoryAllResult{}, err
	}
	result := runAllUsageHistory(ctx, channels, since, until, func(ctx context.Context, channel *storage.Channel, since, until time.Time) error {
		tasklog.Add(ctx, "INFO", "\u5f00\u59cb\u6838\u5bf9\u6e20\u9053 #%d %s", channel.ID, channel.Name)
		report, err := s.RefetchUsageHistoryWithReport(ctx, channel, since, until)
		if err == nil && report != nil {
			tasklog.Add(ctx, "INFO", "\u6e20\u9053 #%d %s \u6838\u5bf9\u5b8c\u6210\uff1a\u4fdd\u5b58 %d \u9875\u3001%d \u6761\u6570\u636e", channel.ID, channel.Name, report.SavedPages, report.SavedRows)
		}
		return err
	})
	for _, item := range result.Errors {
		if item.ChannelID > 0 {
			s.log.Error("channel usage history reconciliation failed", "channel_id", item.ChannelID, "channel", item.ChannelName, "err", item.Err)
		}
	}
	return result, nil
}
