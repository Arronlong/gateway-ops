package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/guliansheng/gateway-ops/internal/notify"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
)

type Service struct {
	rules      *storage.Alerts
	collector  MetricCollector
	actions    ActionExecutor
	dispatcher *notify.Dispatcher
	log        *slog.Logger
}

type PreviewTarget struct {
	TargetType       string     `json:"target_type"`
	StationID        uint       `json:"station_id"`
	TargetExternalID string     `json:"target_external_id"`
	TargetName       string     `json:"target_name"`
	Metric           MetricKind `json:"metric"`
	Actual           float64    `json:"actual"`
	Violated         bool       `json:"violated"`
	Unknown          bool       `json:"unknown"`
	Summary          string     `json:"summary"`
}

type PreviewSummary struct {
	Matched int             `json:"matched"`
	Targets []PreviewTarget `json:"targets"`
}

func NewService(rules *storage.Alerts, collector MetricCollector, actions ActionExecutor, dispatcher *notify.Dispatcher, log *slog.Logger) *Service {
	return &Service{rules: rules, collector: collector, actions: actions, dispatcher: dispatcher, log: log}
}

func (s *Service) ListRules(filter storage.AlertRuleFilter) ([]storage.AlertRule, error) {
	return s.rules.ListRules(filter)
}

func (s *Service) FindRule(id uint) (*storage.AlertRule, error) { return s.rules.FindRule(id) }

func (s *Service) CreateRule(rule *storage.AlertRule, channelIDs []uint) error {
	if err := validateRule(rule); err != nil {
		return err
	}
	return s.rules.CreateRule(rule, channelIDs)
}

func (s *Service) UpdateRule(rule *storage.AlertRule, channelIDs []uint) error {
	if err := validateRule(rule); err != nil {
		return err
	}
	return s.rules.WithMutationLock(context.Background(), func(repo *storage.Alerts) error { return repo.UpdateRule(rule, channelIDs) })
}

func (s *Service) DeleteRule(id uint) error {
	return s.rules.WithMutationLock(context.Background(), func(repo *storage.Alerts) error { return repo.DeleteRule(id) })
}

func (s *Service) SetRuleEnabled(ctx context.Context, id uint, enabled bool) error {
	return s.rules.WithMutationLock(ctx, func(repo *storage.Alerts) error {
		rule, err := repo.FindRule(id)
		if err != nil {
			return err
		}
		rule.Enabled = enabled
		return repo.UpdateRule(rule, rule.ChannelIDs)
	})
}

func (s *Service) ListEvents(filter storage.AlertEventFilter) ([]storage.AlertEvent, error) {
	return s.rules.ListEvents(filter)
}

func (s *Service) FindEvent(id uint) (*storage.AlertEvent, error) { return s.rules.FindEvent(id) }

func (s *Service) ListActionLogs(eventID uint) ([]storage.AlertActionLog, error) {
	return s.rules.ListActionLogs(eventID)
}

func (s *Service) AcknowledgeEvent(id uint, at time.Time) error {
	if _, err := s.rules.FindEvent(id); err != nil {
		return err
	}
	return s.rules.AcknowledgeEvent(id, at)
}

func (s *Service) RecoverEvent(id uint, at time.Time) error {
	return s.rules.WithMutationLock(context.Background(), func(repo *storage.Alerts) error {
		locked := *s
		locked.rules = repo
		return locked.closeEvent(id, at)
	})
}

func (s *Service) closeEvent(id uint, at time.Time) error {
	event, err := s.rules.FindEvent(id)
	if err != nil {
		return err
	}
	if event.RecoveredAt != nil {
		return nil
	}
	state, err := s.rules.GetOrCreateState(storage.AlertStateKey{RuleID: event.RuleID, TargetType: event.TargetType, RelayStationID: event.RelayStationID, TargetExternalID: event.TargetExternalID})
	if err != nil {
		return err
	}
	state.State = StateRecovered
	state.ConsecutiveViolations = 0
	state.ConsecutiveRecoveries = 0
	state.HistoryJSON = "[]"
	state.ManualIntervened = true
	if err := s.rules.SaveState(state); err != nil {
		return err
	}
	return s.rules.RecoverEvent(id, at, "人工关闭事件；未变更远端状态，后续异常仍会重新触发")
}

func (s *Service) Summary(now time.Time) (storage.AlertSummary, error) { return s.rules.Summary(now) }

func (s *Service) TestRule(ctx context.Context, ruleID uint, now time.Time) ([]PreviewTarget, error) {
	rule, err := s.rules.FindRule(ruleID)
	if err != nil {
		return nil, err
	}
	return s.PreviewRule(ctx, *rule, now)
}

func (s *Service) PreviewRule(ctx context.Context, rule storage.AlertRule, now time.Time) ([]PreviewTarget, error) {
	if err := validateRule(&rule); err != nil {
		return nil, err
	}
	var condition ConditionConfig
	if err := json.Unmarshal([]byte(rule.ConditionJSON), &condition); err != nil {
		return nil, err
	}
	spec := condition.RuleSpec(MetricKind(rule.Metric))
	targets, err := s.collector.Collect(ctx, rule, condition, now)
	if err != nil {
		return nil, err
	}
	preview := make([]PreviewTarget, 0, len(targets))
	for _, target := range targets {
		if spec.Metric == MetricRateChange && rule.ID != 0 {
			states, err := s.rules.ListRuleStates(rule.ID)
			if err != nil {
				return nil, err
			}
			for _, state := range states {
				if state.RelayStationID == target.StationID && state.TargetExternalID == target.TargetExternalID {
					if state.BaselineValue != nil {
						target.Baseline = &Baseline{Value: *state.BaselineValue}
					}
					break
				}
			}
		}
		result := EvaluateCondition(spec, target.Observation, target.Baseline)
		preview = append(preview, PreviewTarget{TargetType: rule.TargetType, StationID: target.StationID, TargetExternalID: target.TargetExternalID, TargetName: target.TargetName, Metric: target.Metric, Actual: result.Actual, Violated: result.Violated, Unknown: result.Unknown, Summary: result.Summary})
	}
	return preview, nil
}

func (s *Service) PreviewRuleSummary(ctx context.Context, rule storage.AlertRule, now time.Time) (PreviewSummary, error) {
	targets, err := s.PreviewRule(ctx, rule, now)
	if err != nil {
		return PreviewSummary{}, err
	}
	previewCount := min(len(targets), 10)
	return PreviewSummary{Matched: len(targets), Targets: targets[:previewCount]}, nil
}

func (s *Service) EvaluateAll(ctx context.Context, now time.Time) error {
	return s.rules.WithEvaluationLock(ctx, func(repo *storage.Alerts) error {
		locked := *s
		locked.rules = repo
		return locked.evaluateAllLocked(ctx, now)
	})
}

func (s *Service) evaluateAllLocked(ctx context.Context, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	enabled := true
	rules, err := s.rules.ListRules(storage.AlertRuleFilter{Enabled: &enabled})
	if err != nil {
		return err
	}
	var errs []error
	if err := s.retryRecoveryNotifications(ctx); err != nil {
		errs = append(errs, err)
	}
	for _, rule := range rules {
		if err := s.evaluateRule(ctx, rule, now); err != nil {
			errs = append(errs, fmt.Errorf("rule %d: %w", rule.ID, err))
		}
	}
	if err := s.rules.PruneSamples(now.Add(-8 * 24 * time.Hour)); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Service) evaluateRule(ctx context.Context, rule storage.AlertRule, now time.Time) error {
	var condition ConditionConfig
	if err := json.Unmarshal([]byte(rule.ConditionJSON), &condition); err != nil {
		return fmt.Errorf("decode condition: %w", err)
	}
	spec := condition.RuleSpec(MetricKind(rule.Metric))
	var recovery RecoverySpec
	if err := json.Unmarshal([]byte(rule.RecoveryJSON), &recovery); err != nil {
		return err
	}
	if recovery.Threshold != nil {
		spec.RecoveryThreshold = recovery.Threshold
	}
	if recovery.Consecutive > 0 {
		spec.RecoveryConsecutive = recovery.Consecutive
	}
	targets, err := s.collector.Collect(ctx, rule, condition, now)
	if err != nil {
		return err
	}
	var errs []error
	for _, target := range targets {
		if err := s.evaluateTarget(ctx, rule, condition, spec, target, now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) evaluateTarget(ctx context.Context, rule storage.AlertRule, condition ConditionConfig, spec RuleSpec, target TargetObservation, now time.Time) error {
	state, err := s.rules.GetOrCreateState(storage.AlertStateKey{
		RuleID: rule.ID, TargetType: rule.TargetType, RelayStationID: target.StationID, TargetExternalID: target.TargetExternalID,
	})
	if err != nil {
		return err
	}
	baseline := target.Baseline
	if spec.Metric == MetricRateChange && target.Observation.Valid {
		if state.BaselineValue == nil {
			value := target.Observation.Value
			state.BaselineValue = &value
		}
		baseline = &Baseline{Value: *state.BaselineValue}
	}
	result := EvaluateCondition(spec, target.Observation, baseline)
	transition := Transition(StateInput{Previous: *state, Condition: result, TriggerConsecutive: spec.TriggerConsecutive, RecoveryConsecutive: spec.RecoveryConsecutive, Now: now, ObservedAt: &target.Observation.ObservedAt, HitWindow: spec.HitWindow})
	*state = transition.Next
	if spec.Metric == MetricRateChange && !result.Unknown && !result.Violated && (state.State == StateNormal || state.State == StateRecovered) {
		value := target.Observation.Value
		state.BaselineValue = &value
	}
	if err := s.rules.SaveState(state); err != nil {
		return err
	}
	if target.Observation.Valid && !result.Unknown {
		if err := s.rules.SaveMetricSample(&storage.AlertMetricSample{
			RuleID: rule.ID, TargetType: rule.TargetType, RelayStationID: target.StationID,
			TargetExternalID: target.TargetExternalID, Metric: rule.Metric, ObservedAt: target.Observation.ObservedAt,
			Value: target.Observation.Value, RequestCount: target.Observation.RequestCount,
		}); err != nil {
			return err
		}
	}

	if !result.Unknown && result.Violated && (transition.OpenEvent || transition.Next.State == StateFiring) {
		return s.handleFiring(ctx, rule, target, result, state, transition.OpenEvent, now)
	}
	if !result.Unknown && !result.Violated && !result.InRecoveryBand && (transition.RecoverEvent || state.State == StateRecovered) {
		return s.handleRecovery(ctx, rule, target, result, state, now)
	}
	return nil
}

func (s *Service) handleFiring(ctx context.Context, rule storage.AlertRule, target TargetObservation, result ConditionResult, state *storage.AlertRuleState, opened bool, now time.Time) error {
	event := &storage.AlertEvent{
		RuleID: rule.ID, TargetType: rule.TargetType, RelayStationID: target.StationID, TargetExternalID: target.TargetExternalID,
		Severity: rule.Severity, Metric: rule.Metric, State: storage.AlertStateFiring, ActualValue: &result.Actual,
		Summary: result.Summary, StartedAt: now, FiredAt: &now, UpdatedAt: now,
	}
	var config ConditionConfig
	_ = json.Unmarshal([]byte(rule.ConditionJSON), &config)
	event.ThresholdValue = &config.Threshold
	sample, err := json.Marshal(target.Observation)
	if err != nil {
		return err
	}
	event.SampleJSON = string(sample)
	if existing, err := s.rules.FindOpenEvent(rule.ID, rule.TargetType, target.StationID, target.TargetExternalID); err != nil {
		return err
	} else if existing != nil {
		event.ID = existing.ID
		event.StartedAt = existing.StartedAt
		event.FiredAt = existing.FiredAt
	} else {
		state.ManualIntervened = false
		state.AutomaticActionJSON = "{}"
		if err := s.rules.SaveState(state); err != nil {
			return err
		}
	}
	if err := s.rules.UpsertOpenEvent(event); err != nil {
		return err
	}
	if err := s.runFiringAction(ctx, rule, event, state, now); err != nil && s.log != nil {
		s.log.Warn("alert action failed", "event_id", event.ID, "error", err)
	}
	return s.sendEventNotification(ctx, rule, target, event, false)
}

func (s *Service) handleRecovery(ctx context.Context, rule storage.AlertRule, target TargetObservation, result ConditionResult, state *storage.AlertRuleState, now time.Time) error {
	event, err := s.rules.FindOpenEvent(rule.ID, rule.TargetType, target.StationID, target.TargetExternalID)
	if err != nil || event == nil {
		return err
	}
	var recovery RecoverySpec
	if err := json.Unmarshal([]byte(rule.RecoveryJSON), &recovery); err != nil {
		return err
	}
	ready, err := s.runRecoveryAction(ctx, rule, event, state, recovery, now)
	if err != nil {
		return err
	}
	if !ready {
		state.State = StateRecovering
		return s.rules.SaveState(state)
	}
	event.State = StateRecovered
	event.ActualValue = &result.Actual
	event.Summary = result.Summary
	event.RecoveredAt = &now
	event.UpdatedAt = now
	event.RecoveryNotificationPending = true
	if err := s.rules.SaveEvent(event); err != nil {
		return err
	}
	if err := s.sendEventNotification(ctx, rule, target, event, true); err != nil {
		return err
	}
	return s.rules.RecoveryNotificationSent(event.ID)
}

// 恢复落库与通知投递分离，单渠道失败不阻塞下一次异常的自动化。
func (s *Service) retryRecoveryNotifications(ctx context.Context) error {
	events, err := s.rules.PendingRecoveryNotifications()
	if err != nil {
		return err
	}
	var errs []error
	for _, event := range events {
		rule, err := s.rules.FindRule(event.RuleID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if clearErr := s.rules.RecoveryNotificationSent(event.ID); clearErr != nil {
				errs = append(errs, clearErr)
			}
			continue
		}
		if err == nil {
			target := TargetObservation{StationID: event.RelayStationID, TargetExternalID: event.TargetExternalID, TargetName: event.TargetExternalID}
			err = s.sendEventNotification(ctx, *rule, target, &event, true)
		}
		if err == nil {
			err = s.rules.RecoveryNotificationSent(event.ID)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) sendEventNotification(ctx context.Context, rule storage.AlertRule, target TargetObservation, event *storage.AlertEvent, recovered bool) error {
	channels, err := s.rules.ListRuleChannelIDs(rule.ID)
	if err != nil || len(channels) == 0 {
		return err
	}
	if s.dispatcher == nil {
		return errors.New("通知分发器未配置")
	}
	subject := rule.Name
	var config ConditionConfig
	_ = json.Unmarshal([]byte(rule.ConditionJSON), &config)
	logs, err := s.rules.ListActionLogs(event.ID)
	if err != nil {
		return err
	}
	actionSummary := "未配置自动化动作"
	if len(logs) > 0 {
		actionSummary = ""
		for _, entry := range logs {
			actionSummary += fmt.Sprintf("%s：%s %s；", alertLabel(entry.Action), alertLabel(entry.Status), entry.ErrorMessage)
		}
	}
	body := fmt.Sprintf("规则：%s\n等级：%s\n中转站：%d\n目标：%s（%s）\n指标：%s\n状态：%s\n实际值：%.4f\n阈值：%.4f\n窗口：%d 秒\n触发时间：%s\n%s\n自动化：%s\n事件：/alerts?event=%d（事件 #%d）", rule.Name, alertLabel(rule.Severity), target.StationID, target.TargetName, target.TargetExternalID, alertLabel(event.Metric), alertLabel(string(event.State)), valueOrZero(event.ActualValue), valueOrZero(event.ThresholdValue), config.WindowSeconds, event.StartedAt.In(time.FixedZone("Asia/Shanghai", 28800)).Format(time.RFC3339), event.Summary, actionSummary, event.ID, event.ID)
	if recovered {
		subject = "已恢复：" + subject
	}
	eventType := storage.EventAlertFiring
	if recovered {
		eventType = storage.EventAlertRecovered
	}
	return s.dispatcher.DispatchToChannels(ctx, channels, notify.Message{Event: eventType, Subject: subject, Body: body, AlertEventID: event.ID, AlertRuleID: rule.ID, AlertCooldown: time.Duration(rule.CooldownSeconds) * time.Second})
}

func alertLabel(value string) string {
	labels := map[string]string{"info": "提示", "warning": "警告", "critical": "严重", "rate_change": "倍率变化", "channel_rate_change": "倍率变化", "balance_low": "渠道余额低", "account_spend": "账号消耗异常", "account_stability": "账号稳定性异常", "user_spend": "用户消耗异常", "user_multi_ip": "用户多 IP", "user_balance_low": "用户余额低", "firing": "触发中", "recovering": "恢复中", "recovered": "已恢复", "disable_account_scheduling": "自动停止账号调度", "enable_account_scheduling": "自动恢复账号调度", "disable_user": "自动停用用户", "enable_user": "自动启用用户", "prepared": "准备执行", "succeeded": "执行成功", "skipped": "已跳过", "failed": "执行失败", "uncertain": "结果待核对"}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}

func inverseAction(kind ActionKind) (ActionKind, bool) {
	switch kind {
	case ActionDisableAccountScheduling:
		return ActionEnableAccountScheduling, true
	case ActionDisableUser:
		return ActionEnableUser, true
	default:
		return "", false
	}
}

func valueOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func validateRuleDefaults(rule *storage.AlertRule) error {
	if rule == nil || strings.TrimSpace(rule.Name) == "" {
		return errors.New("alert rule name is required")
	}
	if rule.TargetType != string(TargetChannel) && rule.TargetType != string(TargetRelayAccount) && rule.TargetType != string(TargetRelayUser) {
		return fmt.Errorf("unsupported alert target type: %s", rule.TargetType)
	}
	if rule.Metric == "" {
		return errors.New("alert rule metric is required")
	}
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	if rule.ConditionJSON == "" {
		rule.ConditionJSON = "{}"
	}
	if rule.ActionJSON == "" {
		rule.ActionJSON = "{}"
	}
	if rule.RecoveryJSON == "" {
		rule.RecoveryJSON = "{}"
	}
	if rule.ScopeJSON == "" {
		rule.ScopeJSON = "{}"
	}
	if rule.CooldownSeconds < 0 {
		return errors.New("cooldown cannot be negative")
	}
	return nil
}
