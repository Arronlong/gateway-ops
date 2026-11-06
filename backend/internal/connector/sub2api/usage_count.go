package sub2api

import (
	"context"
	"fmt"
	"time"

	"github.com/guliansheng/gateway-ops/internal/connector"
)

func (c *Client) CountUsage(ctx context.Context, ch *connector.Channel, session *connector.AuthSession, query connector.UsageQuery) (int64, error) {
	if query.Until.IsZero() || (!query.Since.IsZero() && !query.Since.Before(query.Until)) {
		return 0, fmt.Errorf("invalid usage count window")
	}
	endDay := query.Until.UTC().Truncate(24 * time.Hour)
	// 上游只支持 UTC 日期过滤，首尾日逐条筛选，中间完整日期使用总数。
	start := query.Since
	var count int64
	if !start.IsZero() {
		if !start.Before(endDay) {
			return c.countUsageWindow(ctx, ch, session, start, query.Until, false)
		}
		startDay := start.UTC().Truncate(24 * time.Hour)
		if !start.Equal(startDay) {
			boundary := startDay.Add(24 * time.Hour)
			n, err := c.countUsageWindow(ctx, ch, session, start, boundary, false)
			if err != nil {
				return 0, err
			}
			count += n
			start = boundary
		}
	}
	if start.IsZero() || start.Before(endDay) {
		n, err := c.countUsageWindow(ctx, ch, session, start, endDay, true)
		if err != nil {
			return 0, err
		}
		count += n
	}
	if endDay.Before(query.Until) {
		n, err := c.countUsageWindow(ctx, ch, session, endDay, query.Until, false)
		if err != nil {
			return 0, err
		}
		count += n
	}
	return count, nil
}

func (c *Client) countUsageWindow(ctx context.Context, ch *connector.Channel, session *connector.AuthSession, since, until time.Time, wholeDays bool) (int64, error) {
	query := connector.UsageQuery{Since: since, Until: until, Page: 1, PageSize: 1}
	first, err := c.ListUsage(ctx, ch, session, query)
	if err != nil {
		return 0, err
	}
	if first == nil || !first.TotalKnown || first.Total < int64(first.RawCount) || first.RawCount != len(first.Records) || (first.Total > 0 && first.RawCount == 0) {
		return 0, fmt.Errorf("upstream did not return a reliable request total; full history enumeration is disabled")
	}
	if wholeDays || first.Total == 0 {
		return first.Total, nil
	}
	if first.PageSize != 1 || first.RawCount != 1 {
		return 0, fmt.Errorf("upstream does not support bounded partial-day counts")
	}
	dayStart := until.Add(-time.Nanosecond).UTC().Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	if first.Records[0].CreatedAt.Before(dayStart) || !first.Records[0].CreatedAt.Before(dayEnd) {
		return 0, fmt.Errorf("upstream ignored the UTC day filter")
	}
	stamps := map[int64]time.Time{1: first.Records[0].CreatedAt}
	readStamp := func(index int64) (time.Time, error) {
		if stamp, ok := stamps[index]; ok {
			return stamp, nil
		}
		query.Page = int(index)
		page, err := c.ListUsage(ctx, ch, session, query)
		if err != nil {
			return time.Time{}, err
		}
		if page == nil || !page.TotalKnown || page.Total != first.Total || page.PageSize != 1 || len(page.Records) != 1 || page.RawCount != 1 {
			return time.Time{}, fmt.Errorf("upstream usage changed during boundary count")
		}
		stamp := page.Records[0].CreatedAt
		if stamp.Before(dayStart) || !stamp.Before(dayEnd) {
			return time.Time{}, fmt.Errorf("upstream ignored the UTC day filter")
		}
		for otherIndex, otherStamp := range stamps {
			if (otherIndex < index && otherStamp.Before(stamp)) || (otherIndex > index && otherStamp.After(stamp)) {
				return time.Time{}, fmt.Errorf("upstream ignored descending usage order")
			}
		}
		stamps[index] = stamp
		return stamp, nil
	}
	countAtOrAfter := func(boundary time.Time) (int64, error) {
		if !boundary.After(dayStart) {
			return first.Total, nil
		}
		if !boundary.Before(dayEnd) {
			return 0, nil
		}
		low, high := int64(1), first.Total+1
		for low < high {
			middle := low + (high-low)/2
			stamp, err := readStamp(middle)
			if err != nil {
				return 0, err
			}
			if !stamp.Before(boundary) {
				low = middle + 1
			} else {
				high = middle
			}
		}
		return low - 1, nil
	}
	startCount, err := countAtOrAfter(since)
	if err != nil {
		return 0, err
	}
	endCount, err := countAtOrAfter(until)
	return startCount - endCount, err
}
