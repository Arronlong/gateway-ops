package monitor

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestUsageHistoryWalkSkipsHealthyMonthsWeeksAndDays(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	anomaly := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	stamps := []time.Time{start.Add(12 * time.Hour), anomaly.Add(-7 * 24 * time.Hour), anomaly, anomaly.Add(9 * 24 * time.Hour)}
	local := []int64{3, 4, 1, 5}
	remote := []int64{3, 4, 2, 5}
	var checked, repaired []usageHistoryWindow
	report := &UsageHistoryReport{}
	walker := usageHistoryWalker{Report: report, Count: func(ctx context.Context, w usageHistoryWindow) (usageWindowCounts, error) {
		checked = append(checked, w)
		var counts usageWindowCounts
		for i, stamp := range stamps {
			if !stamp.Before(w.Since) && stamp.Before(w.Until) {
				counts.Local += local[i]
				counts.Remote += remote[i]
			}
		}
		return counts, nil
	}, Repair: func(ctx context.Context, w usageHistoryWindow, expected int64) error {
		repaired = append(repaired, w)
		for i, stamp := range stamps {
			if !stamp.Before(w.Since) && stamp.Before(w.Until) {
				local[i] = remote[i]
			}
		}
		return nil
	}}
	for month := 0; month < 2; month++ {
		if _, err := walker.walk(context.Background(), usageHistoryWindow{Since: start.AddDate(0, month, 0), Until: start.AddDate(0, month+1, 0)}, "month", nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(repaired) != 1 || !repaired[0].Since.Equal(anomaly.Truncate(24*time.Hour)) || repaired[0].Until.Sub(repaired[0].Since) != 24*time.Hour {
		t.Fatalf("repaired=%#v", repaired)
	}
	augustChecks := 0
	for _, w := range checked {
		if w.Since.Before(start.AddDate(0, 1, 0)) {
			augustChecks++
		}
	}
	if augustChecks != 1 || report.SkippedWindows == 0 || report.AnomalousDays != 1 || report.RepairedDays != 1 {
		t.Fatalf("healthy month was expanded or report invalid: %#v checks=%d", report, augustChecks)
	}
	repaired = nil
	for month := 0; month < 2; month++ {
		if _, err := walker.walk(context.Background(), usageHistoryWindow{Since: start.AddDate(0, month, 0), Until: start.AddDate(0, month+1, 0)}, "month", nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(repaired) != 0 {
		t.Fatal("healthy history was fetched again")
	}
}

func TestUsageHistoryWalkNeverRepairsUnknownOrMissingUpstreamHistory(t *testing.T) {
	window := usageHistoryWindow{Since: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)}
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			calls := 0
			walker := usageHistoryWalker{Report: &UsageHistoryReport{}, Count: func(context.Context, usageHistoryWindow) (usageWindowCounts, error) {
				if unknown {
					return usageWindowCounts{}, fmt.Errorf("count unavailable")
				}
				return usageWindowCounts{Local: 10, Remote: 0}, nil
			}, Repair: func(context.Context, usageHistoryWindow, int64) error { calls++; return nil }}
			if _, err := walker.walk(context.Background(), window, "month", nil); err == nil || calls != 0 {
				t.Fatalf("error=%v repairs=%d", err, calls)
			}
		})
	}
}

func TestUsageHistoryPartitionsRespectClippedCalendarBoundaries(t *testing.T) {
	since := time.Date(2026, 10, 29, 16, 30, 0, 0, time.UTC)
	until := time.Date(2026, 11, 4, 15, 59, 0, 0, time.UTC)
	for _, level := range []string{"month", "week", "day"} {
		parts := splitUsageHistoryWindow(usageHistoryWindow{Since: since, Until: until}, level)
		cursor := since
		for _, part := range parts {
			if !part.Since.Equal(cursor) || !part.Since.Before(part.Until) || part.Until.After(until) {
				t.Fatalf("invalid %s partition: %#v", level, parts)
			}
			cursor = part.Until
		}
		if !cursor.Equal(until) {
			t.Fatalf("%s partition omitted time", level)
		}
	}
}

func TestDenseNewAPIAnomalyNarrowsToHoursBeforeDownloading(t *testing.T) {
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	stamp := day.Add(10 * time.Hour)
	repaired := false
	walker := usageHistoryWalker{Report: &UsageHistoryReport{}, SplitDense: true, Count: func(_ context.Context, window usageHistoryWindow) (usageWindowCounts, error) {
		if !stamp.Before(window.Since) && stamp.Before(window.Until) {
			local := int64(0)
			if repaired {
				local = 5000
			}
			return usageWindowCounts{Local: local, Remote: 5000}, nil
		}
		return usageWindowCounts{}, nil
	}, Repair: func(_ context.Context, window usageHistoryWindow, expected int64) error {
		if window.Until.Sub(window.Since) > time.Hour {
			t.Fatalf("dense interval was not narrowed: %#v", window)
		}
		if expected != 5000 {
			t.Fatal("incorrect expected count")
		}
		repaired = true
		return nil
	}}
	if _, err := walker.walk(context.Background(), usageHistoryWindow{Since: day, Until: day.Add(24 * time.Hour)}, "month", nil); err != nil {
		t.Fatal(err)
	}
	if !repaired || walker.Report.RepairedDays != 1 {
		t.Fatal("dense day was not repaired")
	}
}
