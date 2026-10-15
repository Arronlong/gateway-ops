package alert

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func EvaluateCondition(rule RuleSpec, observation Observation, baseline *Baseline) ConditionResult {
	if !observation.Valid {
		return ConditionResult{Unknown: true, Summary: observation.Reason}
	}
	if observation.RequestCount < rule.MinRequests {
		return ConditionResult{Unknown: true, Actual: float64(observation.RequestCount), Summary: "insufficient samples"}
	}

	switch rule.Metric {
	case MetricRateChange:
		if baseline == nil || baseline.Value == 0 || math.IsNaN(baseline.Value) || math.IsInf(baseline.Value, 0) {
			return ConditionResult{Unknown: true, Summary: "missing rate baseline"}
		}
		change := math.Abs((observation.Value - baseline.Value) / baseline.Value * 100)
		return ConditionResult{Violated: change >= rule.Threshold, Actual: change, Summary: fmt.Sprintf("rate changed %.2f%%", change)}
	case MetricAccountSpend, MetricUserSpend:
		violated := observation.Value >= rule.Threshold
		if rule.BaselineMultiplier != nil {
			if baseline == nil || baseline.Value < 0 {
				return ConditionResult{Unknown: true, Actual: observation.Value, Summary: "missing spend baseline"}
			}
			violated = violated && observation.Value >= baseline.Value**rule.BaselineMultiplier
		}
		return ConditionResult{Violated: violated, Actual: observation.Value, Summary: fmt.Sprintf("spend %.4f", observation.Value)}
	case MetricUserMultiIP:
		trusted := make(map[string]struct{}, len(rule.TrustedIPs))
		for _, ip := range rule.TrustedIPs {
			trusted[strings.TrimSpace(ip)] = struct{}{}
		}
		seen := make(map[string]struct{}, len(observation.IPAddresses))
		for _, ip := range observation.IPAddresses {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if _, ok := trusted[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
		}
		count := len(seen)
		return ConditionResult{Violated: float64(count) >= rule.Threshold, Actual: float64(count), Summary: fmt.Sprintf("distinct IPs %d", count)}
	default:
		return ConditionResult{Unknown: true, Summary: "unsupported metric"}
	}
}

func Percentile(values []float64, percentile float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	if percentile <= 0 {
		return copyValues[0], true
	}
	if percentile >= 100 {
		return copyValues[len(copyValues)-1], true
	}
	index := int(math.Ceil(percentile/100*float64(len(copyValues)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copyValues) {
		index = len(copyValues) - 1
	}
	return copyValues[index], true
}
