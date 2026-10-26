package alert

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func activeAlertTargetIDs(states []storage.AlertRuleState, targetType TargetType, stationID uint) map[string]struct{} {
	active := make(map[string]struct{})
	for _, state := range states {
		if state.TargetType != string(targetType) || state.RelayStationID != stationID {
			continue
		}
		if isActiveAlertState(state.State) {
			active[state.TargetExternalID] = struct{}{}
		}
	}
	return active
}

func isActiveAlertState(state storage.AlertLifecycleState) bool {
	switch state {
	case storage.AlertStateFiring, storage.AlertStateRecovering:
		return true
	default:
		return false
	}
}

func (c *Collector) collectChannels(ctx context.Context, rule storage.AlertRule, scope ScopeConfig, condition ConditionConfig, now time.Time) ([]TargetObservation, error) {
	if c.channels == nil {
		return nil, fmt.Errorf("channel alert source is unavailable")
	}
	activeTargets := map[string]struct{}{}
	if c.alerts != nil && rule.ID > 0 {
		states, err := c.alerts.ListRuleStates(rule.ID)
		if err != nil {
			return nil, err
		}
		activeTargets = activeAlertTargetIDs(states, TargetChannel, 0)
	}
	channels, err := c.channels.List()
	if err != nil {
		return nil, err
	}
	channelIDs := make([]uint, 0, len(channels))
	for _, channel := range channels {
		channelIDs = append(channelIDs, channel.ID)
	}
	modelsByChannel := map[uint][]string{}
	if c.usage != nil {
		modelsByChannel, err = c.usage.DistinctModelsByChannels(channelIDs)
		if err != nil {
			return nil, err
		}
	}
	if c.rates != nil {
		rateModels, err := c.rates.DistinctOwnedModelsByChannels(channelIDs)
		if err != nil {
			return nil, err
		}
		for channelID, models := range rateModels {
			seen := make(map[string]struct{}, len(modelsByChannel[channelID])+len(models))
			for _, model := range modelsByChannel[channelID] {
				seen[model] = struct{}{}
			}
			for _, model := range models {
				if _, exists := seen[model]; !exists {
					modelsByChannel[channelID] = append(modelsByChannel[channelID], model)
				}
			}
		}
	}
	targets := make(map[string]struct{}, len(scope.TargetIDs))
	for _, id := range scope.TargetIDs {
		targets[strings.TrimSpace(id)] = struct{}{}
	}
	result := make([]TargetObservation, 0, len(channels))
	for _, channel := range channels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := strconv.FormatUint(uint64(channel.ID), 10)
		if len(targets) > 0 && !hasScopeTarget(targets, key) {
			continue
		}
		attributes := channelScopeAttributes(channel, modelsByChannel[channel.ID], now)
		if c.scopeNeedsChannelMetrics(scope) && c.usage != nil {
			samples, err := c.usage.RecentSamples(channel.ID, 60)
			if err != nil {
				return nil, err
			}
			derived := deriveChannelMetrics(samples, now)
			if derived.Score != nil {
				attributes["score"] = strconv.FormatFloat(*derived.Score, 'f', -1, 64)
			}
			if derived.CacheHitRate != nil {
				attributes["cache_hit_rate"] = strconv.FormatFloat(*derived.CacheHitRate, 'f', -1, 64)
			}
		}
		if !scopeMatches(TargetChannel, scope, attributes) && !hasScopeTarget(activeTargets, key) {
			continue
		}
		metric := MetricKind(rule.Metric)
		switch metric {
		case MetricBalanceLow:
			if !channel.MonitorEnabled {
				if hasScopeTarget(activeTargets, key) {
					result = append(result, TargetObservation{TargetExternalID: key, TargetName: channel.Name, Metric: metric, Observation: Observation{Reason: "渠道已停止监控，余额状态未知"}})
				}
				continue
			}
			value, observedAt, ok := channelBalance(channel, now)
			observation := Observation{ObservedAt: observedAt, Value: value, Valid: ok}
			if !ok {
				observation.Reason = "渠道余额未开启监控，或余额数据缺失、已过期"
			}
			result = append(result, TargetObservation{TargetExternalID: key, TargetName: channel.Name, Metric: metric, Observation: observation})
		case MetricChannelRateChange:
			observation, err := c.channelRateChange(channel.ID, scope, attributes, condition, now)
			if err != nil {
				return nil, err
			}
			result = append(result, TargetObservation{TargetExternalID: key, TargetName: channel.Name, Metric: metric, Observation: observation})
		}
	}
	return result, nil
}

func (c *Collector) scopeNeedsChannelMetrics(scope ScopeConfig) bool {
	for _, filter := range scope.Filters {
		if filter.Field == "score" || filter.Field == "cache_hit_rate" {
			return true
		}
	}
	return false
}

func channelScopeAttributes(channel storage.Channel, models []string, now time.Time) map[string]string {
	attributes := map[string]string{
		"name": channel.Name, "type": string(channel.Type),
		"monitor_enabled": strconv.FormatBool(channel.MonitorEnabled),
		"balance_mode":    string(channel.BalanceMode),
		"model":           strings.Join(models, "\n"),
	}
	if balance, _, ok := channelBalance(channel, now); ok {
		attributes["balance"] = strconv.FormatFloat(balance, 'f', -1, 64)
	}
	return attributes
}

func channelBalance(channel storage.Channel, now time.Time) (float64, time.Time, bool) {
	if channel.BalanceMode == storage.BalanceModeManual {
		if channel.LastBalance != nil && finite(*channel.LastBalance) {
			at := channel.UpdatedAt
			if channel.LastBalanceAt != nil {
				at = *channel.LastBalanceAt
			}
			return *channel.LastBalance, at.UTC(), true
		}
		if finite(channel.ManualBalance) {
			return channel.ManualBalance, channel.UpdatedAt.UTC(), true
		}
		return 0, time.Time{}, false
	}
	if channel.LastError != "" || channel.LastBalance == nil || channel.LastBalanceAt == nil || !finite(*channel.LastBalance) || !fresh(*channel.LastBalanceAt, now, 10*time.Minute) {
		return 0, time.Time{}, false
	}
	return *channel.LastBalance, channel.LastBalanceAt.UTC(), true
}

func (c *Collector) channelRateChange(channelID uint, scope ScopeConfig, baseAttributes map[string]string, condition ConditionConfig, now time.Time) (Observation, error) {
	if c.rates == nil {
		return Observation{Reason: "渠道倍率采集不可用"}, nil
	}
	window := time.Duration(condition.WindowSeconds) * time.Second
	if window <= 0 {
		window = 15 * time.Minute
	}
	changes, err := c.rates.ListOwnedChangesByChannelSince(channelID, now.Add(-window))
	if err != nil {
		return Observation{}, err
	}
	mode, direction := condition.RateMode, condition.Direction
	if mode == "" {
		mode = "percent"
	}
	if direction == "" {
		direction = "any"
	}
	best := 0.0
	observedAt := now
	uncomputableBaseline := false
	for _, change := range changes {
		attributes := make(map[string]string, len(baseAttributes)+1)
		for key, value := range baseAttributes {
			attributes[key] = value
		}
		attributes["model"] = change.ModelName
		if !scopeMatches(TargetChannel, scope, attributes) {
			continue
		}
		if change.OldRatio == nil || (mode != "absolute" && *change.OldRatio == 0) {
			uncomputableBaseline = true
			continue
		}
		delta := change.NewRatio - *change.OldRatio
		switch direction {
		case "increase":
			if delta < 0 {
				continue
			}
		case "decrease":
			delta = -delta
		default:
			delta = math.Abs(delta)
		}
		if mode != "absolute" {
			delta = delta / math.Abs(*change.OldRatio) * 100
		}
		if delta > best {
			best = delta
			observedAt = change.ChangedAt.UTC()
		}
	}
	if uncomputableBaseline && best < condition.Threshold {
		return Observation{Reason: "部分模型倍率基线为零或缺失，无法完整计算变化"}, nil
	}
	return Observation{ObservedAt: observedAt, Value: best, Valid: true}, nil
}

type channelDerivedMetrics struct {
	Score        *float64
	CacheHitRate *float64
}

func deriveChannelMetrics(samples []storage.ChannelUsageSample, now time.Time) channelDerivedMetrics {
	cutoff := now.Add(-45 * time.Second)
	windowStart := cutoff.Add(-time.Hour)
	selected := make([]storage.ChannelUsageSample, 0, 60)
	for _, sample := range samples {
		if !sample.CreatedAt.Before(windowStart) && !sample.CreatedAt.After(cutoff) {
			selected = append(selected, sample)
			if len(selected) == 60 {
				break
			}
		}
	}
	if len(selected) == 0 {
		return channelDerivedMetrics{}
	}
	var attempts, successes int
	var cacheInput, cacheRead, cacheCreation float64
	firstLatency, durationLatency := make([]float64, 0, len(selected)), make([]float64, 0, len(selected))
	latencyScores := make([]float64, 0, len(selected))
	for _, sample := range selected {
		input, read, creation := nonNegativeInt(sample.InputTokens), nonNegativeInt(sample.CacheReadTokens), nonNegativeInt(sample.CacheCreationTokens)
		if input+read+creation > 0 {
			cacheInput += input
			cacheRead += read
			cacheCreation += creation
		}
		if !sample.SuccessKnown {
			continue
		}
		attempts++
		if !sample.Success {
			continue
		}
		successes++
		latencyScoresForSample := make([]float64, 0, 2)
		if sample.FirstTokenMS != nil && *sample.FirstTokenMS > 0 {
			value := float64(*sample.FirstTokenMS)
			firstLatency = append(firstLatency, value)
			latencyScoresForSample = append(latencyScoresForSample, latencyScore(value, 10_000, 30_000))
		}
		if sample.DurationMS != nil && *sample.DurationMS > 0 {
			value := float64(*sample.DurationMS)
			durationLatency = append(durationLatency, value)
			latencyScoresForSample = append(latencyScoresForSample, latencyScore(value, 60_000, 180_000))
		}
		if len(latencyScoresForSample) > 0 {
			latencyScores = append(latencyScores, averageFloat(latencyScoresForSample))
		}
	}
	metrics := channelDerivedMetrics{}
	cacheTotal := cacheInput + cacheRead + cacheCreation
	if cacheTotal > 0 {
		value := cacheRead / cacheTotal * 100
		metrics.CacheHitRate = &value
	}
	if successes == 0 || len(latencyScores) == 0 || metrics.CacheHitRate == nil {
		return metrics
	}
	consistency := make([]float64, 0, 2)
	if value, ok := latencyConsistencyScore(firstLatency); ok {
		consistency = append(consistency, value)
	}
	if value, ok := latencyConsistencyScore(durationLatency); ok {
		consistency = append(consistency, value)
	}
	if len(consistency) == 0 {
		return metrics
	}
	longestFailure, run := 0, 0
	for _, sample := range selected {
		if !sample.SuccessKnown {
			continue
		}
		if sample.Success {
			run = 0
		} else {
			run++
			if run > longestFailure {
				longestFailure = run
			}
		}
	}
	stability := averageFloat(consistency) * math.Max(0, 1-float64(max(0, longestFailure-1))/float64(attempts-1))
	parts := []float64{float64(successes) / float64(attempts) * 100, stability, *metrics.CacheHitRate, averageFloat(latencyScores)}
	value := math.Round(averageFloat(parts))
	metrics.Score = &value
	return metrics
}

func latencyScore(value, warn, bad float64) float64 {
	if value <= warn {
		return 100
	}
	if value >= bad {
		return 0
	}
	return 100 * (bad - value) / (bad - warn)
}

func latencyConsistencyScore(values []float64) (float64, bool) {
	if len(values) < 2 {
		return 0, false
	}
	mean := averageFloat(values)
	if mean <= 0 {
		return 0, false
	}
	squared := make([]float64, 0, len(values))
	for _, value := range values {
		squared = append(squared, (value-mean)*(value-mean))
	}
	deviation := math.Sqrt(averageFloat(squared))
	return 100 / (1 + deviation/mean), true
}

func averageFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func nonNegativeInt(value int64) float64 {
	if value < 0 {
		return 0
	}
	return float64(value)
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func hasScopeTarget(targets map[string]struct{}, key string) bool {
	_, ok := targets[key]
	return ok
}

func accountScopeAttributes(account storage.RelayAccount, station storage.RelayStation) map[string]string {
	attributes := map[string]string{
		"station_name": station.Name, "station_type": station.Type, "name": account.Name,
		"identity": account.Identity, "platform": account.Platform, "type": account.Type,
		"status": account.Status, "schedulable": strconv.FormatBool(account.Schedulable),
		"account_plan": account.AccountPlan, "model_type": account.ModelType,
	}
	if account.ExpiresAt != nil {
		attributes["expires_at"] = account.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if account.LastUsedAt != nil {
		attributes["last_used_at"] = account.LastUsedAt.UTC().Format(time.RFC3339)
	}
	return attributes
}

func userScopeAttributes(user storage.RelayUserSnapshot, station storage.RelayStation) map[string]string {
	attributes := map[string]string{
		"station_name": station.Name, "station_type": station.Type,
		"username": user.Username, "email": user.Email, "role": user.Role,
		"status": user.Status, "balance": strconv.FormatFloat(user.Balance, 'f', -1, 64),
		"concurrency": strconv.Itoa(user.Concurrency), "rpm_limit": strconv.Itoa(user.RPMLimit),
		"created_at": user.CreatedAt.UTC().Format(time.RFC3339),
	}
	if user.LastUsedAt != nil {
		attributes["last_used_at"] = user.LastUsedAt.UTC().Format(time.RFC3339)
	}
	return attributes
}

func userDisplayName(user storage.RelayUserSnapshot) string {
	if strings.TrimSpace(user.Username) != "" {
		return user.Username
	}
	if strings.TrimSpace(user.Email) != "" {
		return user.Email
	}
	return strconv.FormatInt(user.ID, 10)
}
