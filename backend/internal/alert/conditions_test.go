package alert

import (
	"testing"
	"time"
)

func TestEvaluateRateChangeUsesAbsolutePercentage(t *testing.T) {
	rule := RuleSpec{Metric: MetricRateChange, Threshold: 20}
	baseline := &Baseline{Value: 1.0, ObservedAt: time.Now().UTC()}
	result := EvaluateCondition(rule, Observation{Value: 1.25, Valid: true}, baseline)
	if !result.Violated {
		t.Fatalf("expected rate change to violate threshold, got %+v", result)
	}
}

func TestEvaluateSpendWindowRequiresMinimumRequests(t *testing.T) {
	rule := RuleSpec{Metric: MetricAccountSpend, Threshold: 30, MinRequests: 3}
	result := EvaluateCondition(rule, Observation{Value: 40, RequestCount: 2, Valid: true}, nil)
	if !result.Unknown {
		t.Fatalf("expected insufficient samples to be unknown, got %+v", result)
	}
}

func TestEvaluateSpendBaselineNeedsAmountAndMultiplier(t *testing.T) {
	multiplier := 3.0
	rule := RuleSpec{Metric: MetricAccountSpend, Threshold: 10, BaselineMultiplier: &multiplier}
	baseline := &Baseline{Value: 5, ObservedAt: time.Now().UTC()}
	result := EvaluateCondition(rule, Observation{Value: 12, RequestCount: 4, Valid: true}, baseline)
	if result.Violated {
		t.Fatalf("expected baseline amount and multiplier to both be required, got %+v", result)
	}
	result = EvaluateCondition(rule, Observation{Value: 16, RequestCount: 4, Valid: true}, baseline)
	if !result.Violated {
		t.Fatalf("expected baseline amount and multiplier to violate, got %+v", result)
	}
}

func TestEvaluateMultiIPIgnoresTrustedIPs(t *testing.T) {
	rule := RuleSpec{Metric: MetricUserMultiIP, Threshold: 3, TrustedIPs: []string{"10.0.0.1"}}
	result := EvaluateCondition(rule, Observation{IPAddresses: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4"}, Valid: true}, nil)
	if !result.Violated {
		t.Fatalf("expected multi-ip rule to violate after trusted IP removal, got %+v", result)
	}
}
