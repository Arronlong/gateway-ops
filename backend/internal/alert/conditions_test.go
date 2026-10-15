package alert

import (
	"math"
	"testing"
	"time"
)

func TestAllMetricThresholdsAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		metric           MetricKind
		value, threshold float64
		violated         bool
	}{
		{MetricBalanceLow, 5, 10, true}, {MetricBalanceLow, 15, 10, false},
		{MetricAccountStability, 20, 10, true}, {MetricAccountStability, 5, 10, false},
	} {
		r := EvaluateCondition(RuleSpec{Metric: tc.metric, Threshold: tc.threshold}, Observation{Value: tc.value, Valid: true}, nil)
		if r.Unknown || r.Violated != tc.violated {
			t.Fatalf("threshold: %+v => %+v", tc, r)
		}
	}
	if r := EvaluateCondition(RuleSpec{Metric: MetricAccountSpend}, Observation{Value: math.NaN(), Valid: true}, nil); !r.Unknown {
		t.Fatal("non-finite sample accepted")
	}
}

func TestMultiIPNormalizesIPv6AndCIDR(t *testing.T) {
	r := EvaluateCondition(RuleSpec{Metric: MetricUserMultiIP, Threshold: 2, TrustedIPs: []string{"10.0.0.0/8"}}, Observation{Valid: true, IPAddresses: []string{"10.1.2.3", "2001:db8::1", "2001:0db8:0:0::1"}}, nil)
	if r.Unknown || r.Actual != 1 || r.Violated {
		t.Fatalf("IP normalization: %+v", r)
	}
}

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

func TestSpendBaselineDoesNotBypassRecoveryThreshold(t *testing.T) {
	multiplier, recoverAt := 3.0, 5.0
	rule := RuleSpec{Metric: MetricAccountSpend, Threshold: 10, RecoveryThreshold: &recoverAt, BaselineMultiplier: &multiplier}
	result := EvaluateCondition(rule, Observation{Value: 12, RequestCount: 4, Valid: true}, &Baseline{Value: 5})
	if result.Violated || result.Unknown || !result.InRecoveryBand {
		t.Fatalf("baseline comparison bypassed recovery threshold: %+v", result)
	}
}

func TestEvaluateMultiIPIgnoresTrustedIPs(t *testing.T) {
	rule := RuleSpec{Metric: MetricUserMultiIP, Threshold: 3, TrustedIPs: []string{"10.0.0.1"}}
	result := EvaluateCondition(rule, Observation{IPAddresses: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4"}, Valid: true}, nil)
	if !result.Violated {
		t.Fatalf("expected multi-ip rule to violate after trusted IP removal, got %+v", result)
	}
}
