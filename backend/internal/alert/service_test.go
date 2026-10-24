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
