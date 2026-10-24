package alert

import (
	"strconv"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func fresh(observed, now time.Time, maxAge time.Duration) bool {
	return !observed.IsZero() && !observed.After(now.Add(5*time.Second)) && now.Sub(observed) <= maxAge
}

func (c *Collector) usageWatermark(stationID uint, window time.Duration, now time.Time) (time.Time, bool) {
	state, err := c.stations.UsageSyncState(stationID)
	if err != nil || state.ForwardSyncedAt == nil || state.BackfilledAt == nil || !fresh(*state.ForwardSyncedAt, now, 3*time.Minute) || state.BackfilledAt.After(state.ForwardSyncedAt.Add(-window)) {
		return time.Time{}, false
	}
	return *state.ForwardSyncedAt, true
}

// spendBaseline 使用过去七天的同时间窗口均值，缺少三天完整历史时返回未知。
func (c *Collector) spendBaseline(rule storage.AlertRule, stationID uint, targetID string, spec RuleSpec, now time.Time) *Baseline {
	if spec.BaselineMultiplier == nil {
		return nil
	}
	state, err := c.stations.UsageSyncState(stationID)
	if err != nil || state.BackfilledAt == nil {
		return nil
	}
	var sum float64
	days := 0
	for day := 1; day <= 7; day++ {
		end := now.Add(-time.Duration(day) * 24 * time.Hour)
		start := end.Add(-spec.Window)
		if state.BackfilledAt.After(start) {
			continue
		}
		var events []storage.RelayUsageEvent
		if rule.TargetType == string(TargetRelayUser) {
			events, err = c.stations.ListUsageEventsByUserWindow(stationID, targetID, start, end)
		} else {
			id, _ := strconv.ParseInt(targetID, 10, 64)
			events, err = c.stations.ListUsageEventsByAccountWindow(stationID, id, start, end)
		}
		if err != nil {
			return nil
		}
		for _, event := range events {
			if rule.TargetType == string(TargetRelayUser) {
				sum += event.ActualCost
			} else if event.AccountStatsCost != nil {
				sum += *event.AccountStatsCost
			} else if event.AccountRateMultiplier != nil {
				sum += event.TotalCost * *event.AccountRateMultiplier
			} else {
				return nil
			}
		}
		days++
	}
	if days < 3 {
		return nil
	}
	return &Baseline{Value: sum / float64(days), ObservedAt: now}
}
