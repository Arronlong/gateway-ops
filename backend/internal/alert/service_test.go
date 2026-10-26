package alert

import (
	"testing"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestValidateRuleRejectsMissingName(t *testing.T) {
	rule := &storage.AlertRule{TargetType: string(TargetRelayAccount), Metric: string(MetricRateChange)}
	if err := validateRule(rule); err == nil {
		t.Fatal("expected missing name to be rejected")
	}
}

func TestValidateRuleAppliesJSONDefaults(t *testing.T) {
	rule := &storage.AlertRule{Name: "rate", TargetType: string(TargetRelayAccount), Metric: string(MetricRateChange), ConditionJSON: `{"threshold":20}`, CooldownSeconds: 1800}
	if err := validateRule(rule); err != nil {
		t.Fatal(err)
	}
	if rule.ConditionJSON == "{}" || rule.ActionJSON != "{}" || rule.RecoveryJSON != "{}" || rule.ScopeJSON != "{}" {
		t.Fatalf("defaults = %#v", rule)
	}
}

func TestValidateChannelAndUserBalanceRulesUseTheirOwnTargets(t *testing.T) {
	channelRule := &storage.AlertRule{Name: "渠道余额", TargetType: string(TargetChannel), Metric: string(MetricBalanceLow), ScopeJSON: `{"filters":[{"field":"monitor_enabled","operator":"eq","value":"true"}]}`, ConditionJSON: `{"threshold":0,"window_seconds":900}`, ActionJSON: `{}`, RecoveryJSON: `{}`, CooldownSeconds: 1800}
	if err := validateRule(channelRule); err != nil {
		t.Fatalf("valid channel balance rule rejected: %v", err)
	}
	userRule := &storage.AlertRule{Name: "用户余额", TargetType: string(TargetRelayUser), Metric: string(MetricUserBalanceLow), ScopeJSON: `{"station_ids":[7]}`, ConditionJSON: `{"threshold":0,"recovery_threshold":5,"window_seconds":900}`, ActionJSON: `{}`, RecoveryJSON: `{}`, CooldownSeconds: 1800}
	if err := validateRule(userRule); err != nil {
		t.Fatalf("valid user balance rule rejected: %v", err)
	}
	wrongTarget := *channelRule
	wrongTarget.TargetType = string(TargetRelayAccount)
	wrongTarget.Metric = string(MetricChannelRateChange)
	if err := validateRule(&wrongTarget); err == nil {
		t.Fatal("channel balance metric must not be created for relay accounts")
	}
}

func TestInverseActionOnlyRecoversAutomaticDisableActions(t *testing.T) {
	if inverse, ok := inverseAction(ActionDisableAccountScheduling); !ok || inverse != ActionEnableAccountScheduling {
		t.Fatalf("account inverse = %q, %v", inverse, ok)
	}
	if inverse, ok := inverseAction(ActionDisableUser); !ok || inverse != ActionEnableUser {
		t.Fatalf("user inverse = %q, %v", inverse, ok)
	}
	if _, ok := inverseAction(ActionEnableUser); ok {
		t.Fatal("enable action must not create another recovery action")
	}
}

func TestAlertLabelNamesNewMetrics(t *testing.T) {
	if got := alertLabel("channel_rate_change"); got != "\u500d\u7387\u53d8\u5316" {
		t.Fatalf("channel_rate_change label = %q", got)
	}
	if got := alertLabel("user_balance_low"); got != "\u7528\u6237\u4f59\u989d\u4f4e" {
		t.Fatalf("user_balance_low label = %q", got)
	}
}
