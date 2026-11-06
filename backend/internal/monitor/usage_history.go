package monitor

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type usageHistoryWindow struct{ Since, Until time.Time }
type usageWindowCounts struct{ Local, Remote int64 }
type UsageWindowIntegrity struct {
	TotalCapped   bool      `json:"total_capped,omitempty"`
	AccountID     uint      `json:"account_id,omitempty"`
	AccountName   string    `json:"account_name,omitempty"`
	Since         time.Time `json:"since"`
	Until         time.Time `json:"until"`
	Level         string    `json:"level"`
	Status        string    `json:"status"`
	LocalCount    int64     `json:"local_count"`
	UpstreamCount int64     `json:"upstream_count"`
}
type UsageHistoryReport struct {
	SavedPages     int                    `json:"saved_pages"`
	SavedRows      int                    `json:"saved_rows"`
	Result         *UsageIntegrity        `json:"-"`
	CheckedWindows int                    `json:"checked_windows"`
	SkippedWindows int                    `json:"skipped_windows"`
	AnomalousDays  int                    `json:"anomalous_days"`
	RepairedDays   int                    `json:"repaired_days"`
	Windows        []UsageWindowIntegrity `json:"windows,omitempty"`
}
type usageHistoryWalker struct {
	AccountID           uint
	AccountName         string
	Report              *UsageHistoryReport
	Count               func(context.Context, usageHistoryWindow) (usageWindowCounts, error)
	Repair              func(context.Context, usageHistoryWindow, int64) error
	Plan                func(usageHistoryWindow, usageWindowCounts)
	Progress            func(UsageWindowIntegrity)
	SplitDense          bool
	Incremental         bool
	anomalous, repaired map[string]bool
}

func splitUsageHistoryWindow(window usageHistoryWindow, level string) []usageHistoryWindow {
	var result []usageHistoryWindow
	for cursor := window.Since.UTC(); cursor.Before(window.Until); {
		day := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), 0, 0, 0, 0, time.UTC)
		var next time.Time
		switch level {
		case "month":
			next = time.Date(cursor.Year(), cursor.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		case "week":
			next = day.AddDate(0, 0, 7-(int(day.Weekday())+6)%7)
		default:
			next = day.AddDate(0, 0, 1)
		}
		if next.After(window.Until) {
			next = window.Until
		}
		result = append(result, usageHistoryWindow{Since: cursor, Until: next})
		cursor = next
	}
	return result
}

func (walker *usageHistoryWalker) walk(ctx context.Context, window usageHistoryWindow, level string, known *usageWindowCounts) (usageWindowCounts, error) {
	if err := ctx.Err(); err != nil {
		return usageWindowCounts{}, err
	}
	if walker.Report == nil {
		walker.Report = &UsageHistoryReport{}
	}
	var counts usageWindowCounts
	if known != nil {
		counts = *known
	} else {
		var err error
		if walker.Progress != nil {
			walker.Progress(UsageWindowIntegrity{AccountID: walker.AccountID, AccountName: walker.AccountName, Since: window.Since, Until: window.Until, Level: level, Status: "checking"})
		}
		counts, err = walker.Count(ctx, window)
		if err != nil {
			return counts, err
		}
		walker.Report.CheckedWindows++
	}
	if counts.Local < 0 || counts.Remote < 0 {
		return counts, fmt.Errorf("invalid usage counts")
	}
	item := UsageWindowIntegrity{AccountID: walker.AccountID, AccountName: walker.AccountName, Since: window.Since, Until: window.Until, Level: level, Status: "mismatch", LocalCount: counts.Local, UpstreamCount: counts.Remote}
	record := func() {
		if item.Status != "matched" && len(walker.Report.Windows) < 128 {
			walker.Report.Windows = append(walker.Report.Windows, item)
		}
		if walker.Progress != nil {
			walker.Progress(item)
		}
	}
	if counts.Local == counts.Remote {
		walker.Report.SkippedWindows++
		item.Status = "matched"
		record()
		return counts, nil
	}
	dayStart := window.Since.UTC().Truncate(24 * time.Hour)
	leaf := !window.Until.After(dayStart.Add(24 * time.Hour))
	if leaf && walker.SplitDense && counts.Remote > 2000 && window.Until.Sub(window.Since) > time.Hour {
		record()
		middle := window.Since.Add(window.Until.Sub(window.Since) / 2).Truncate(time.Second)
		if walker.Incremental {
			return counts, walker.walkIncrementalParts(ctx, window, counts, []usageHistoryWindow{{Since: window.Since, Until: middle}, {Since: middle, Until: window.Until}}, "hour")
		}
		var childTotal int64
		for _, part := range []usageHistoryWindow{{Since: window.Since, Until: middle}, {Since: middle, Until: window.Until}} {
			child, err := walker.walk(ctx, part, "hour", nil)
			if err != nil {
				return counts, err
			}
			childTotal += child.Remote
		}
		if childTotal != counts.Remote {
			return counts, usageCountConflict("hour", counts.Remote, childTotal)
		}
		return counts, nil
	}
	if leaf {
		item.Level = "day"
		key := dayStart.Format("2006-01-02")
		if walker.anomalous == nil {
			walker.anomalous = make(map[string]bool)
			walker.repaired = make(map[string]bool)
		}
		if !walker.anomalous[key] {
			walker.anomalous[key] = true
			walker.Report.AnomalousDays++
		}
		if walker.Repair != nil {
			if counts.Remote == 0 {
				record()
				return counts, fmt.Errorf("upstream returned no calls for %s; retained existing history", key)
			}
			if err := walker.Repair(ctx, window, counts.Remote); err != nil {
				record()
				return counts, err
			}
			if !walker.repaired[key] {
				walker.repaired[key] = true
				walker.Report.RepairedDays++
			}
			item.Status = "repaired"
		}
		if walker.Plan != nil {
			walker.Plan(window, counts)
		}
		record()
		return counts, nil
	}
	record()
	nextLevel := "day"
	if level == "total" {
		nextLevel = "month"
	}
	if level == "month" {
		nextLevel = "week"
	}
	if walker.Incremental {
		return counts, walker.walkIncrementalParts(ctx, window, counts, splitUsageHistoryWindow(window, nextLevel), nextLevel)
	}
	var childTotal int64
	for _, part := range splitUsageHistoryWindow(window, nextLevel) {
		var existing *usageWindowCounts
		if part.Since.Equal(window.Since) && part.Until.Equal(window.Until) {
			existing = &counts
		}
		child, err := walker.walk(ctx, part, nextLevel, existing)
		if err != nil {
			return counts, err
		}
		childTotal += child.Remote
	}
	if childTotal != counts.Remote {
		return counts, usageCountConflict(nextLevel, counts.Remote, childTotal)
	}
	return counts, nil
}

func usageCountConflict(level string, total, parts int64) error {
	label := map[string]string{"month": "\u6708", "week": "\u5468", "day": "\u5929", "hour": "\u5c0f\u65f6"}[level]
	return fmt.Errorf("%w\uff1a\u603b\u6570 %d \u6761\uff0c\u6309%s\u5408\u8ba1 %d \u6761\uff1b\u8bf7\u91cd\u65b0\u6838\u5bf9\u540e\u518d\u4fee\u590d", errUsageCountConflict, total, label, parts)
}

var errUsageCountConflict = errors.New("\u4e0a\u6e38\u7edf\u8ba1\u4e0d\u4e00\u81f4")
