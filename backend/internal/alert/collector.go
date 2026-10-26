package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

type TargetObservation struct {
	StationID        uint
	TargetExternalID string
	TargetName       string
	Metric           MetricKind
	Observation      Observation
	Baseline         *Baseline
}

type MetricCollector interface {
	Collect(ctx context.Context, rule storage.AlertRule, config ConditionConfig, now time.Time) ([]TargetObservation, error)
}

type Collector struct {
	alerts   *storage.Alerts
	stations *storage.RelayStations
	channels *storage.Channels
	usage    *storage.ChannelUsage
	rates    *storage.Rates
	users    UserSnapshotProvider
}

type UserSnapshotProvider interface {
	AlertUserSnapshots(ctx context.Context, stationID uint) ([]storage.RelayUserSnapshot, error)
}

type CollectorSources struct {
	ChannelUsage *storage.ChannelUsage
	Rates        *storage.Rates
	Users        UserSnapshotProvider
}

func NewCollector(alerts *storage.Alerts, stations *storage.RelayStations, channels *storage.Channels, sources ...CollectorSources) *Collector {
	collector := &Collector{alerts: alerts, stations: stations, channels: channels}
	if len(sources) > 0 {
		collector.usage = sources[0].ChannelUsage
		collector.rates = sources[0].Rates
		collector.users = sources[0].Users
	}
	return collector
}

func (c *Collector) Collect(ctx context.Context, rule storage.AlertRule, config ConditionConfig, now time.Time) ([]TargetObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var scope ScopeConfig
	if strings.TrimSpace(rule.ScopeJSON) != "" && strings.TrimSpace(rule.ScopeJSON) != "{}" {
		if err := json.Unmarshal([]byte(rule.ScopeJSON), &scope); err != nil {
			return nil, fmt.Errorf("decode alert scope: %w", err)
		}
	}
	targetType := TargetType(rule.TargetType)
	if targetType == TargetChannel {
		return c.collectChannels(ctx, rule, scope, config, now)
	}
	stationIDs := scope.StationIDs
	if len(stationIDs) == 0 {
		stations, err := c.stations.List()
		if err != nil {
			return nil, err
		}
		stationIDs = make([]uint, 0, len(stations))
		for _, station := range stations {
			stationIDs = append(stationIDs, station.ID)
		}
	}
	targets := make(map[string]struct{}, len(scope.TargetIDs))
	for _, id := range scope.TargetIDs {
		targets[strings.TrimSpace(id)] = struct{}{}
	}
	metric := MetricKind(rule.Metric)
	spec := config.RuleSpec(metric)
	result := make([]TargetObservation, 0)
	activeTargetsByStation := map[uint]map[string]struct{}{}
	if targetType == TargetRelayAccount && len(scope.Filters) > 0 && c.alerts != nil && rule.ID > 0 {
		states, err := c.alerts.ListRuleStates(rule.ID)
		if err != nil {
			return nil, err
		}
		for _, state := range states {
			if state.TargetType != string(TargetRelayAccount) || !isActiveAlertState(state.State) {
				continue
			}
			if activeTargetsByStation[state.RelayStationID] == nil {
				activeTargetsByStation[state.RelayStationID] = map[string]struct{}{}
			}
			activeTargetsByStation[state.RelayStationID][state.TargetExternalID] = struct{}{}
		}
	}
	for _, stationID := range stationIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		station, err := c.stations.FindByID(stationID)
		if err != nil {
			return nil, err
		}
		if targetType == TargetRelayUser && c.users != nil {
			users, err := c.users.AlertUserSnapshots(ctx, stationID)
			if err != nil {
				return nil, err
			}
			states, err := c.alerts.ListRuleStates(rule.ID)
			if err != nil {
				return nil, err
			}
			activeTargets := activeAlertTargetIDs(states, TargetRelayUser, stationID)
			known := map[string]bool{}
			if metric == MetricUserBalanceLow || len(scope.Filters) > 0 {
				for _, user := range users {
					key := strconv.FormatInt(user.ID, 10)
					if len(targets) > 0 {
						if _, ok := targets[key]; !ok {
							continue
						}
					}
					if !scopeMatches(targetType, scope, userScopeAttributes(user, *station)) {
						continue
					}
					known[key] = true
				}
			}
			for key := range activeTargets {
				known[key] = true
			}
			if metric != MetricUserBalanceLow {
				keys, err := c.stations.ListUsageUserKeys(stationID, now.Add(-spec.Window))
				if err != nil {
					return nil, err
				}
				for _, key := range keys {
					if len(targets) == 0 || hasScopeTarget(targets, key) {
						known[key] = true
					}
				}
				for _, state := range states {
					if state.RelayStationID == stationID {
						known[state.TargetExternalID] = true
					}
				}
				for key := range targets {
					known[key] = true
				}
			}
			userByID := make(map[string]storage.RelayUserSnapshot, len(users))
			for _, user := range users {
				userByID[strconv.FormatInt(user.ID, 10)] = user
			}
			for key := range known {
				if len(targets) > 0 && !hasScopeTarget(targets, key) {
					continue
				}
				if len(scope.Filters) > 0 || metric == MetricUserBalanceLow {
					user, exists := userByID[key]
					if !exists || (!scopeMatches(targetType, scope, userScopeAttributes(user, *station)) && !hasScopeTarget(activeTargets, key)) {
						continue
					}
					if metric == MetricUserBalanceLow {
						observation := Observation{ObservedAt: user.ObservedAt.UTC(), Value: user.Balance, Valid: !user.ObservedAt.IsZero() && fresh(user.ObservedAt, now, 3*time.Minute)}
						if !observation.Valid {
							observation.Reason = "用户余额快照缺失或已过期"
						}
						result = append(result, TargetObservation{StationID: stationID, TargetExternalID: key, TargetName: userDisplayName(user), Metric: metric, Observation: observation})
						continue
					}
				}
				observation, err := c.collectUser(stationID, key, metric, spec, now)
				if err != nil {
					return nil, err
				}
				result = append(result, TargetObservation{StationID: stationID, TargetExternalID: key, TargetName: key, Metric: metric, Observation: observation, Baseline: c.spendBaseline(rule, stationID, key, spec, observation.ObservedAt)})
			}
			continue
		}
		if targetType == TargetRelayUser && (metric == MetricUserBalanceLow || len(scope.Filters) > 0) {
			return nil, fmt.Errorf("user scope filtering requires a station adapter")
		}
		if targetType == TargetRelayUser {
			keys, err := c.stations.ListUsageUserKeys(stationID, now.Add(-spec.Window))
			if err != nil {
				return nil, err
			}
			states, err := c.alerts.ListRuleStates(rule.ID)
			if err != nil {
				return nil, err
			}
			known := map[string]bool{}
			for _, key := range keys {
				known[key] = true
			}
			for _, state := range states {
				if state.RelayStationID == stationID {
					known[state.TargetExternalID] = true
				}
			}
			for key := range targets {
				known[key] = true
			}
			keys = keys[:0]
			for key := range known {
				keys = append(keys, key)
			}
			for _, key := range keys {
				if len(targets) > 0 {
					if _, ok := targets[key]; !ok {
						continue
					}
				}
				observation, err := c.collectUser(stationID, key, metric, spec, now)
				if err != nil {
					return nil, err
				}
				result = append(result, TargetObservation{StationID: stationID, TargetExternalID: key, TargetName: key, Metric: metric, Observation: observation, Baseline: c.spendBaseline(rule, stationID, key, spec, observation.ObservedAt)})
			}
			continue
		}
		accounts, err := c.stations.ListAccounts(stationID)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			key := fmt.Sprintf("%d", account.ExternalID)
			if len(targets) > 0 {
				if _, ok := targets[key]; !ok {
					continue
				}
			}
			if !scopeMatches(targetType, scope, accountScopeAttributes(account, *station)) && !hasScopeTarget(activeTargetsByStation[stationID], key) {
				continue
			}
			observation, err := c.collectAccount(stationID, account, metric, spec, now)
			if err != nil {
				return nil, err
			}
			result = append(result, TargetObservation{StationID: stationID, TargetExternalID: key, TargetName: account.Name, Metric: metric, Observation: observation, Baseline: c.spendBaseline(rule, stationID, key, spec, observation.ObservedAt)})
		}
	}
	return result, nil
}

func (c *Collector) baseline(ruleID uint, targetType string, stationID uint, targetID string, metric MetricKind) *Baseline {
	if c.alerts == nil {
		return nil
	}
	sample, err := c.alerts.LatestMetricSample(ruleID, targetType, stationID, targetID, string(metric))
	if err != nil || sample == nil {
		return nil
	}
	return &Baseline{Value: sample.Value, ObservedAt: sample.ObservedAt}
}

func (c *Collector) collectAccount(stationID uint, account storage.RelayAccount, metric MetricKind, spec RuleSpec, now time.Time) (Observation, error) {
	switch metric {
	case MetricRateChange:
		observedAt := account.RateObservedAt
		if account.RateMultiplier == nil || observedAt == nil || !fresh(*observedAt, now, 3*time.Minute) {
			return Observation{Valid: false, Reason: "倍率样本缺失或已过期"}, nil
		}
		return Observation{ObservedAt: observedAt.UTC(), Value: *account.RateMultiplier, Valid: true}, nil
	case MetricBalanceLow:
		accounts, err := c.channels.ListAccountsForRelayAccount(stationID, account.ExternalID)
		if err != nil {
			return Observation{}, err
		}
		value, ok := lowestAccountBalance(accounts, now, 10*time.Minute)
		if !ok {
			return Observation{Valid: false, Reason: "未关联渠道账号，或余额数据缺失、已过期"}, nil
		}
		var observed time.Time
		for _, linked := range accounts {
			if linked.LastBalanceAt == nil || !fresh(*linked.LastBalanceAt, now, 10*time.Minute) || linked.LastBalance == nil || linked.LastError != "" {
				return Observation{Reason: "部分关联渠道账号余额不可用"}, nil
			}
			if observed.IsZero() || linked.LastBalanceAt.Before(observed) {
				observed = *linked.LastBalanceAt
			}
		}
		return Observation{ObservedAt: observed, Value: value, Valid: true}, nil
	case MetricAccountSpend, MetricAccountStability:
		watermark, ok := c.usageWatermark(stationID, spec.Window, now)
		if !ok {
			return Observation{Reason: "用量采集未完成或已过期"}, nil
		}
		events, err := c.stations.ListUsageEventsByAccountWindow(stationID, account.ExternalID, watermark.Add(-spec.Window), watermark)
		if err != nil {
			return Observation{}, err
		}
		if metric == MetricAccountSpend {
			observation := AggregateUsage(events, watermark)
			observation.Value = 0
			for _, event := range events {
				if event.AccountStatsCost != nil {
					observation.Value += *event.AccountStatsCost
				} else if event.AccountRateMultiplier != nil {
					observation.Value += event.TotalCost * *event.AccountRateMultiplier
				} else {
					return Observation{Reason: "账号成本口径缺失，不能用用户扣费代替"}, nil
				}
			}
			return observation, nil
		}
		return stabilityObservation(events, watermark, spec.StabilityMetric), nil
	default:
		return Observation{Valid: false, Reason: "unsupported account metric"}, nil
	}
}

func (c *Collector) collectUser(stationID uint, userKey string, metric MetricKind, spec RuleSpec, now time.Time) (Observation, error) {
	watermark, ok := c.usageWatermark(stationID, spec.Window, now)
	if !ok {
		return Observation{Reason: "用量采集未完成或已过期"}, nil
	}
	events, err := c.stations.ListUsageEventsByUserWindow(stationID, userKey, watermark.Add(-spec.Window), watermark)
	if err != nil {
		return Observation{}, err
	}
	switch metric {
	case MetricUserSpend:
		return AggregateUsage(events, watermark), nil
	case MetricUserMultiIP:
		observation := AggregateUsage(events, watermark)
		observation.IPAddresses = make([]string, 0, len(events))
		for _, event := range events {
			observation.IPAddresses = append(observation.IPAddresses, event.IPAddress)
		}
		return observation, nil
	default:
		return Observation{Valid: false, Reason: "unsupported user metric"}, nil
	}
}

func stabilityObservation(events []storage.RelayUsageEvent, now time.Time, metric string) Observation {
	metrics := AggregateStability(events)
	value := 0.0
	switch metric {
	case "first_token_p95":
		value = metrics.FirstTokenP95
	case "duration_p95":
		value = metrics.DurationP95
	case "consecutive_failures":
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].StatusCode == 0 {
				return Observation{Reason: "请求状态码缺失"}
			}
			if events[i].StatusCode < 400 {
				break
			}
			value++
		}
	default:
		for _, event := range events {
			if event.StatusCode == 0 {
				return Observation{Reason: "请求状态码缺失，无法判定错误率"}
			}
		}
		if metrics.RequestCount > 0 {
			value = float64(metrics.ErrorCount) / float64(metrics.RequestCount) * 100
		}
	}
	if len(events) == 0 || ((metric == "first_token_p95" || metric == "duration_p95") && value <= 0) {
		return Observation{Reason: "稳定性样本不足"}
	}
	return Observation{ObservedAt: now, Value: value, RequestCount: metrics.RequestCount, ErrorCount: metrics.ErrorCount, Valid: true}
}

func lowestAccountBalance(accounts []storage.ChannelAccount, now time.Time, maxAge time.Duration) (float64, bool) {
	var lowest float64
	found := false
	for _, account := range accounts {
		if account.LastBalance == nil || math.IsNaN(*account.LastBalance) || math.IsInf(*account.LastBalance, 0) || account.LastError != "" || account.LastBalanceAt == nil || !fresh(*account.LastBalanceAt, now, maxAge) {
			continue
		}
		if !found || *account.LastBalance < lowest {
			lowest = *account.LastBalance
			found = true
		}
	}
	return lowest, found
}

type StabilityMetrics struct {
	RequestCount  int64
	ErrorCount    int64
	FirstTokenP95 float64
	DurationP95   float64
}

func AggregateUsage(events []storage.RelayUsageEvent, now time.Time) Observation {
	var total float64
	for _, event := range events {
		total += event.ActualCost
	}
	return Observation{ObservedAt: now, Value: total, RequestCount: int64(len(events)), Valid: true}
}

func FilterUsageByUser(events []storage.RelayUsageEvent, userKey string) []storage.RelayUsageEvent {
	userKey = strings.TrimSpace(userKey)
	filtered := make([]storage.RelayUsageEvent, 0)
	for _, event := range events {
		stableKey := strings.TrimSpace(event.UserExternalID)
		if stableKey == "" {
			stableKey = strings.TrimSpace(event.UserEmail)
		}
		if stableKey == userKey {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func AggregateStability(events []storage.RelayUsageEvent) StabilityMetrics {
	metrics := StabilityMetrics{RequestCount: int64(len(events))}
	firstTokens := make([]float64, 0, len(events))
	durations := make([]float64, 0, len(events))
	for _, event := range events {
		if event.StatusCode >= 400 {
			metrics.ErrorCount++
		}
		if event.FirstTokenMS > 0 {
			firstTokens = append(firstTokens, float64(event.FirstTokenMS))
		}
		if event.DurationMS > 0 {
			durations = append(durations, float64(event.DurationMS))
		}
	}
	metrics.FirstTokenP95, _ = Percentile(firstTokens, 95)
	metrics.DurationP95, _ = Percentile(durations, 95)
	return metrics
}

func LowestFreshBalance(accounts []storage.ChannelAccount, links []storage.ChannelRelayAccountLink, stationID uint, relayAccountExternalID int64, now time.Time, maxAge time.Duration) (float64, bool) {
	accountByID := make(map[uint]storage.ChannelAccount, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	var lowest float64
	found := false
	for _, link := range links {
		if link.RelayStationID != stationID || link.RelayAccountExternalID != relayAccountExternalID {
			continue
		}
		account, ok := accountByID[link.ChannelAccountID]
		if !ok || account.LastBalance == nil || account.LastBalanceAt == nil {
			continue
		}
		if maxAge > 0 && now.Sub(account.LastBalanceAt.UTC()) > maxAge {
			continue
		}
		if !found || *account.LastBalance < lowest {
			lowest = *account.LastBalance
			found = true
		}
	}
	return lowest, found
}
