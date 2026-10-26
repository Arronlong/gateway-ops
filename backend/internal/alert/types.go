package alert

import (
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

type TargetType string

const (
	TargetChannel      TargetType = "channel"
	TargetRelayAccount TargetType = "relay_account"
	TargetRelayUser    TargetType = "relay_user"
)

type MetricKind string

const (
	MetricChannelRateChange MetricKind = "channel_rate_change"
	MetricRateChange        MetricKind = "rate_change"
	MetricBalanceLow        MetricKind = "balance_low"
	MetricAccountSpend      MetricKind = "account_spend"
	MetricAccountStability  MetricKind = "account_stability"
	MetricUserSpend         MetricKind = "user_spend"
	MetricUserMultiIP       MetricKind = "user_multi_ip"
	MetricUserBalanceLow    MetricKind = "user_balance_low"
)

type ActionKind string

const (
	ActionDisableAccountScheduling ActionKind = "disable_account_scheduling"
	ActionEnableAccountScheduling  ActionKind = "enable_account_scheduling"
	ActionDisableUser              ActionKind = "disable_user"
	ActionEnableUser               ActionKind = "enable_user"
)

type ActionSpec struct {
	Kind    ActionKind `json:"kind"`
	Enabled bool       `json:"enabled"`
}

type RecoverySpec struct {
	Enabled     bool     `json:"enabled"`
	Consecutive int      `json:"consecutive"`
	Threshold   *float64 `json:"threshold,omitempty"`
}

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
	RateMode            string
	Direction           string
	HitWindow           int
	StabilityMetric     string
}

type ConditionConfig struct {
	WindowSeconds       int      `json:"window_seconds"`
	Threshold           float64  `json:"threshold"`
	RecoveryThreshold   *float64 `json:"recovery_threshold,omitempty"`
	MinRequests         int64    `json:"min_requests"`
	TriggerConsecutive  int      `json:"trigger_consecutive"`
	RecoveryConsecutive int      `json:"recovery_consecutive"`
	BaselineMultiplier  *float64 `json:"baseline_multiplier,omitempty"`
	TrustedIPs          []string `json:"trusted_ips,omitempty"`
	StabilityMetric     string   `json:"stability_metric,omitempty"`
	RateMode            string   `json:"rate_mode,omitempty"`
	Direction           string   `json:"direction,omitempty"`
	HitWindow           int      `json:"hit_window,omitempty"`
}

func (c ConditionConfig) RuleSpec(metric MetricKind) RuleSpec {
	window := time.Duration(c.WindowSeconds) * time.Second
	if window <= 0 {
		window = 15 * time.Minute
	}
	return RuleSpec{
		Metric: metric, Window: window, Threshold: c.Threshold,
		RecoveryThreshold: c.RecoveryThreshold, MinRequests: c.MinRequests,
		TriggerConsecutive: c.TriggerConsecutive, RecoveryConsecutive: c.RecoveryConsecutive,
		BaselineMultiplier: c.BaselineMultiplier, TrustedIPs: c.TrustedIPs,
		RateMode: c.RateMode, Direction: c.Direction, HitWindow: c.HitWindow, StabilityMetric: c.StabilityMetric,
	}
}

type ScopeConfig struct {
	StationIDs []uint        `json:"station_ids,omitempty"`
	TargetIDs  []string      `json:"target_ids,omitempty"`
	Logic      string        `json:"logic,omitempty"`
	Filters    []ScopeFilter `json:"filters,omitempty"`
}

type ScopeFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type ActionConfig struct {
	Actions []ActionSpec `json:"actions,omitempty"`
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
	Violated       bool
	Unknown        bool
	Actual         float64
	Summary        string
	InRecoveryBand bool
}

type StateInput struct {
	Previous            storage.AlertRuleState
	Condition           ConditionResult
	TriggerConsecutive  int
	RecoveryConsecutive int
	Now                 time.Time
	ObservedAt          *time.Time
	HitWindow           int
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
