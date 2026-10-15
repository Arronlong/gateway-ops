package alert

import (
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

type TargetType string

const (
	TargetRelayAccount TargetType = "relay_account"
	TargetRelayUser    TargetType = "relay_user"
)

type MetricKind string

const (
	MetricRateChange       MetricKind = "rate_change"
	MetricBalanceLow       MetricKind = "balance_low"
	MetricAccountSpend     MetricKind = "account_spend"
	MetricAccountStability MetricKind = "account_stability"
	MetricUserSpend        MetricKind = "user_spend"
	MetricUserMultiIP      MetricKind = "user_multi_ip"
)

type AlertState = storage.AlertLifecycleState

type AlertRuleState = storage.AlertRuleState

const (
	StateNormal     AlertState = "normal"
	StatePending    AlertState = "pending"
	StateFiring     AlertState = "firing"
	StateRecovering AlertState = "recovering"
	StateRecovered  AlertState = "recovered"
	StateUnknown    AlertState = "unknown"
)

type RuleSpec struct {
	Metric              MetricKind
	Window              time.Duration
	Threshold           float64
	RecoveryThreshold   *float64
	MinRequests         int64
	TriggerConsecutive  int
	RecoveryConsecutive int
	BaselineMultiplier  *float64
	TrustedIPs          []string
}

type Observation struct {
	ObservedAt   time.Time
	Value        float64
	RequestCount int64
	ErrorCount   int64
	DistinctIPs  int
	IPAddresses  []string
	Valid        bool
	Reason       string
}

type Baseline struct {
	Value      float64
	ObservedAt time.Time
}

type ConditionResult struct {
	Violated bool
	Unknown  bool
	Actual   float64
	Summary  string
}

type StateInput struct {
	Previous            storage.AlertRuleState
	Condition           ConditionResult
	TriggerConsecutive  int
	RecoveryConsecutive int
	Now                 time.Time
}

type StateTransition struct {
	Next         storage.AlertRuleState
	OpenEvent    bool
	RecoverEvent bool
}

type AlertMessageData struct {
	Event         storage.AlertEvent
	Rule          storage.AlertRule
	ActionResults []storage.AlertActionLog
	Recovered     bool
}
