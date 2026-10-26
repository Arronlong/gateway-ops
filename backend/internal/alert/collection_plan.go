package alert

import (
	"encoding/json"
	"sort"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

// CollectionPlan 无启用规则时不采集；空站点列表仅在 all=true 时代表全部站点。
func (s *Service) CollectionPlan(kind string) (needed bool, stationIDs []uint, err error) {
	enabled := true
	rules, err := s.rules.ListRules(storage.AlertRuleFilter{Enabled: &enabled})
	if err != nil {
		return false, nil, err
	}
	ids := map[uint]bool{}
	all := false
	for _, rule := range rules {
		matches := (kind == "rate" && rule.TargetType == string(TargetRelayAccount) && rule.Metric == string(MetricRateChange)) ||
			(kind == "balance" && rule.Metric == string(MetricBalanceLow)) ||
			(kind == "usage" && (rule.Metric == string(MetricAccountSpend) || rule.Metric == string(MetricAccountStability) || rule.Metric == string(MetricUserSpend) || rule.Metric == string(MetricUserMultiIP)))
		if !matches {
			continue
		}
		needed = true
		var scope ScopeConfig
		if err := json.Unmarshal([]byte(rule.ScopeJSON), &scope); err != nil {
			return false, nil, err
		}
		if len(scope.StationIDs) == 0 {
			all = true
		}
		for _, id := range scope.StationIDs {
			ids[id] = true
		}
	}
	if !all {
		for id := range ids {
			stationIDs = append(stationIDs, id)
		}
		sort.Slice(stationIDs, func(i, j int) bool { return stationIDs[i] < stationIDs[j] })
	}
	return needed, stationIDs, nil
}
