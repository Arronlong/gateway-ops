package alert

import (
	"fmt"
	"math"
	"net/netip"
	"sort"
	"strings"
)

func EvaluateCondition(rule RuleSpec, observation Observation, baseline *Baseline) ConditionResult {
	if !observation.Valid {
		return ConditionResult{Unknown: true, Summary: observation.Reason}
	}
	if math.IsNaN(observation.Value) || math.IsInf(observation.Value, 0) {
		return ConditionResult{Unknown: true, Summary: "指标数据无效"}
	}
	if observation.RequestCount < rule.MinRequests {
		return ConditionResult{Unknown: true, Actual: float64(observation.RequestCount), Summary: "窗口内请求样本不足"}
	}

	switch rule.Metric {
	case MetricChannelRateChange:
		return thresholdResult(rule, observation.Value, "渠道倍率变化")
	case MetricRateChange:
		if baseline == nil || (baseline.Value == 0 && rule.RateMode != "absolute") || math.IsNaN(baseline.Value) || math.IsInf(baseline.Value, 0) {
			return ConditionResult{Unknown: true, Summary: "尚无有效倍率基线"}
		}
		change := observation.Value - baseline.Value
		if rule.RateMode != "absolute" {
			change = change / math.Abs(baseline.Value) * 100
		}
		switch rule.Direction {
		case "increase":
		case "decrease":
			change = -change
		default:
			change = math.Abs(change)
		}
		return thresholdResult(rule, change, "倍率变化")
	case MetricBalanceLow:
		return thresholdResult(rule, observation.Value, "渠道余额")
	case MetricUserBalanceLow:
		return thresholdResult(rule, observation.Value, "用户余额")
	case MetricAccountStability:
		return thresholdResult(rule, observation.Value, "稳定性指标")
	case MetricAccountSpend, MetricUserSpend:
		violated := observation.Value >= rule.Threshold
		if rule.BaselineMultiplier != nil {
			if baseline == nil || baseline.Value <= 0 || math.IsNaN(baseline.Value) || math.IsInf(baseline.Value, 0) {
				return ConditionResult{Unknown: true, Actual: observation.Value, Summary: "历史基线不足：至少需要三个完整的历史同时间窗口"}
			}
			violated = violated && observation.Value >= baseline.Value**rule.BaselineMultiplier
		}
		result := thresholdResult(rule, observation.Value, "窗口消耗")
		result.Violated = violated
		if !violated && rule.RecoveryThreshold != nil {
			result.InRecoveryBand = observation.Value > *rule.RecoveryThreshold
		}
		return result
	case MetricUserMultiIP:
		seen := make(map[netip.Addr]struct{}, len(observation.IPAddresses))
		for _, raw := range observation.IPAddresses {
			ip, err := netip.ParseAddr(strings.TrimSpace(raw))
			if err != nil {
				return ConditionResult{Unknown: true, Summary: "来源 IP 数据缺失或格式无效"}
			}
			ip = ip.Unmap()
			trusted := false
			for _, allow := range rule.TrustedIPs {
				if prefix, err := netip.ParsePrefix(allow); err == nil && prefix.Contains(ip) {
					trusted = true
				}
				if addr, err := netip.ParseAddr(allow); err == nil && addr.Unmap() == ip {
					trusted = true
				}
			}
			if !trusted {
				seen[ip] = struct{}{}
			}
		}
		count := len(seen)
		return thresholdResult(rule, float64(count), "非白名单来源 IP 数")
	default:
		return ConditionResult{Unknown: true, Summary: "当前适配器不支持此指标"}
	}
}

func thresholdResult(rule RuleSpec, value float64, label string) ConditionResult {
	violated := value >= rule.Threshold
	recovered := !violated
	isBalance := rule.Metric == MetricBalanceLow || rule.Metric == MetricUserBalanceLow
	if isBalance {
		violated = value <= rule.Threshold
		recovered = !violated
	}
	if rule.RecoveryThreshold != nil {
		recovered = value <= *rule.RecoveryThreshold
		if isBalance {
			recovered = value >= *rule.RecoveryThreshold
		}
	}
	return ConditionResult{Violated: violated, InRecoveryBand: !violated && !recovered, Actual: value, Summary: fmt.Sprintf("%s %.4f，触发阈值 %.4f", label, value, rule.Threshold)}
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
