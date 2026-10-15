package alert

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func decodeConfig(raw string, value any) error {
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		return errors.New("规则配置必须是 JSON 对象")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func validateRule(rule *storage.AlertRule) error {
	if err := validateRuleDefaults(rule); err != nil {
		return err
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if len([]rune(rule.Name)) > 128 || len([]rune(rule.Remark)) > 512 {
		return errors.New("规则名称或备注超过长度限制")
	}
	valid := map[MetricKind]TargetType{MetricRateChange: TargetRelayAccount, MetricBalanceLow: TargetRelayAccount, MetricAccountSpend: TargetRelayAccount, MetricAccountStability: TargetRelayAccount, MetricUserSpend: TargetRelayUser, MetricUserMultiIP: TargetRelayUser}
	if target, ok := valid[MetricKind(rule.Metric)]; !ok || string(target) != rule.TargetType {
		return errors.New("指标与目标类型不匹配")
	}
	if rule.Severity != "info" && rule.Severity != "warning" && rule.Severity != "critical" {
		return errors.New("预警等级无效")
	}
	var config ConditionConfig
	if err := decodeConfig(rule.ConditionJSON, &config); err != nil {
		return fmt.Errorf("判定配置无效: %w", err)
	}
	if config.WindowSeconds == 0 {
		config.WindowSeconds = 900
	}
	if config.TriggerConsecutive == 0 {
		config.TriggerConsecutive = 3
	}
	if config.RecoveryConsecutive == 0 {
		config.RecoveryConsecutive = 3
	}
	if config.WindowSeconds < 60 || config.WindowSeconds > 86400 || config.TriggerConsecutive < 1 || config.TriggerConsecutive > 100 || config.RecoveryConsecutive < 1 || config.RecoveryConsecutive > 100 || config.MinRequests < 0 || config.MinRequests > 1000000 {
		return errors.New("窗口应为 60–86400 秒，连续次数为 1–100，最小样本数不能为负")
	}
	if config.HitWindow < 0 || config.HitWindow > 100 || (config.HitWindow > 0 && config.HitWindow < config.TriggerConsecutive) {
		return errors.New("最近 M 次评估中命中 N 次时，M 必须大于等于 N，且不超过 100")
	}
	if config.Threshold < 0 || math.IsNaN(config.Threshold) || math.IsInf(config.Threshold, 0) {
		return errors.New("阈值必须是非负有限数")
	}
	if rule.Metric != string(MetricBalanceLow) && config.Threshold == 0 {
		return errors.New("此指标的触发阈值必须大于零")
	}
	if rule.Metric == string(MetricUserMultiIP) && (config.Threshold < 2 || math.Trunc(config.Threshold) != config.Threshold) {
		return errors.New("IP 数量阈值必须是大于等于 2 的整数")
	}
	if config.StabilityMetric == "" {
		config.StabilityMetric = "error_rate"
	}
	if config.StabilityMetric != "error_rate" && config.StabilityMetric != "first_token_p95" && config.StabilityMetric != "duration_p95" && config.StabilityMetric != "consecutive_failures" {
		return errors.New("稳定性指标无效")
	}
	if rule.Metric == string(MetricAccountStability) && config.StabilityMetric == "error_rate" && config.Threshold > 100 {
		return errors.New("错误率阈值不能超过 100%")
	}
	if config.RateMode != "" && config.RateMode != "percent" && config.RateMode != "absolute" {
		return errors.New("倍率比较模式无效")
	}
	if config.Direction != "" && config.Direction != "any" && config.Direction != "increase" && config.Direction != "decrease" {
		return errors.New("倍率变化方向无效")
	}
	if config.BaselineMultiplier != nil {
		if rule.Metric != string(MetricAccountSpend) && rule.Metric != string(MetricUserSpend) {
			return errors.New("仅消耗指标可以使用历史基线")
		}
		if *config.BaselineMultiplier <= 1 || math.IsInf(*config.BaselineMultiplier, 0) || math.IsNaN(*config.BaselineMultiplier) {
			return errors.New("历史基线倍数必须大于 1")
		}
	}
	for _, allow := range config.TrustedIPs {
		if _, err := netip.ParseAddr(allow); err != nil {
			if _, prefixErr := netip.ParsePrefix(allow); prefixErr != nil {
				return errors.New("白名单必须是有效的 IP 或 CIDR")
			}
		}
	}
	var scope ScopeConfig
	if err := decodeConfig(rule.ScopeJSON, &scope); err != nil {
		return err
	}
	if len(scope.StationIDs) > 500 || len(scope.TargetIDs) > 500 {
		return errors.New("规则目标过多")
	}
	for _, id := range scope.StationIDs {
		if id == 0 {
			return errors.New("中转站 ID 无效")
		}
	}
	if len(scope.TargetIDs) > 0 && len(scope.StationIDs) != 1 {
		return errors.New("指定目标 ID 时必须只选择一个中转站，避免跨站 ID 混淆")
	}
	var actions ActionConfig
	if err := decodeConfig(rule.ActionJSON, &actions); err != nil {
		return err
	}
	active := 0
	var action ActionKind
	for _, spec := range actions.Actions {
		if spec.Kind != ActionDisableAccountScheduling && spec.Kind != ActionEnableAccountScheduling && spec.Kind != ActionDisableUser && spec.Kind != ActionEnableUser {
			return errors.New("自动化动作无效")
		}
		if (strings.HasSuffix(string(spec.Kind), "user")) != (rule.TargetType == string(TargetRelayUser)) {
			return errors.New("自动化动作与目标不匹配")
		}
		if spec.Enabled {
			active++
			action = spec.Kind
		}
	}
	if active > 1 {
		return errors.New("同一条规则只能启用一个状态变更动作")
	}
	if active > 0 {
		for _, id := range scope.TargetIDs {
			if value, err := strconv.ParseInt(id, 10, 64); err != nil || value <= 0 {
				return errors.New("自动化目标必须使用有效的远端数字 ID，不能使用邮箱")
			}
		}
	}
	var recovery RecoverySpec
	if err := decodeConfig(rule.RecoveryJSON, &recovery); err != nil {
		return err
	}
	if recovery.Consecutive < 0 || recovery.Consecutive > 100 {
		return errors.New("恢复连续次数必须为 1–100，或留空继承条件设置")
	}
	if recovery.Enabled && (action == ActionEnableUser || action == ActionEnableAccountScheduling) {
		return errors.New("启用动作不能再配置反向自动恢复")
	}
	if recovery.Threshold != nil {
		config.RecoveryThreshold = recovery.Threshold
	}
	if config.RecoveryThreshold != nil {
		value := *config.RecoveryThreshold
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || (rule.Metric == string(MetricBalanceLow) && value <= config.Threshold) || (rule.Metric != string(MetricBalanceLow) && value >= config.Threshold) {
			return errors.New("恢复阈值需要留出滞回区间：余额高于触发阈值，其余指标低于触发阈值")
		}
	}
	if rule.CooldownSeconds < 60 || rule.CooldownSeconds > 604800 {
		return errors.New("通知冷却时间必须为 60 秒到 7 天")
	}
	data, _ := json.Marshal(config)
	rule.ConditionJSON = string(data)
	return nil
}
