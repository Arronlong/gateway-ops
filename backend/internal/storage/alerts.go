package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Alerts struct{ db *gorm.DB }

var alertEvaluationMu sync.Mutex

// WithMutationLock 将人工修改与评估串行化，调用方得到明确的成功或失败结果。
func (r *Alerts) WithMutationLock(ctx context.Context, fn func(*Alerts) error) error {
	alertEvaluationMu.Lock()
	defer alertEvaluationMu.Unlock()
	if r.db.Dialector.Name() != "postgres" {
		return fn(r)
	}
	return r.db.WithContext(ctx).Connection(func(db *gorm.DB) error {
		db = db.Session(&gorm.Session{NewDB: true})
		if err := db.Exec("SELECT pg_advisory_lock(?)", int64(719420261015)).Error; err != nil {
			return err
		}
		defer releaseAlertLock(db)
		return fn(&Alerts{db: db})
	})
}

// WithEvaluationLock 在数据库会话上串行化评估，避免多实例重复执行动作。
func (r *Alerts) WithEvaluationLock(ctx context.Context, fn func(*Alerts) error) error {
	if !alertEvaluationMu.TryLock() {
		return nil
	}
	defer alertEvaluationMu.Unlock()
	if r.db.Dialector.Name() != "postgres" {
		return fn(r)
	}
	return r.db.WithContext(ctx).Connection(func(db *gorm.DB) error {
		db = db.Session(&gorm.Session{NewDB: true})
		var locked bool
		if err := db.Raw("SELECT pg_try_advisory_lock(?)", int64(719420261015)).Scan(&locked).Error; err != nil {
			return err
		}
		if !locked {
			return nil
		}
		defer releaseAlertLock(db)
		return fn(&Alerts{db: db})
	})
}

// 请求取消后仍需释放会话锁，避免带锁连接返回连接池。
func releaseAlertLock(db *gorm.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db.WithContext(ctx).Exec("SELECT pg_advisory_unlock(?)", int64(719420261015))
}

func NewAlerts(db *gorm.DB) *Alerts { return &Alerts{db: db} }

type AlertRuleFilter struct {
	Enabled    *bool
	TargetType string
	Metric     string
}

type AlertEventFilter struct {
	State      string
	TargetType string
	StationID  uint
	Limit      int
	Offset     int
	Severity   string
	From       *time.Time
	To         *time.Time
}

type AlertStateKey struct {
	RuleID           uint
	TargetType       string
	RelayStationID   uint
	TargetExternalID string
}

func (r *Alerts) ListRules(filter AlertRuleFilter) ([]AlertRule, error) {
	query := r.db.Model(&AlertRule{}).Order("id ASC")
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if filter.TargetType != "" {
		query = query.Where("target_type = ?", filter.TargetType)
	}
	if filter.Metric != "" {
		query = query.Where("metric = ?", filter.Metric)
	}
	list := make([]AlertRule, 0)
	if err := query.Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		ids, err := r.ListRuleChannelIDs(list[i].ID)
		if err != nil {
			return nil, err
		}
		list[i].ChannelIDs = ids
	}
	return list, nil
}

func (r *Alerts) FindRule(id uint) (*AlertRule, error) {
	var rule AlertRule
	if err := r.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	ids, err := r.ListRuleChannelIDs(id)
	if err != nil {
		return nil, err
	}
	rule.ChannelIDs = ids
	return &rule, nil
}

func (r *Alerts) CreateRule(rule *AlertRule, channelIDs []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rule).Error; err != nil {
			return err
		}
		return replaceRuleChannels(tx, rule.ID, channelIDs)
	})
}

func (r *Alerts) UpdateRule(rule *AlertRule, channelIDs []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing AlertRule
		if err := tx.First(&existing, rule.ID).Error; err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&AlertEvent{}).Where("rule_id = ? AND recovered_at IS NULL", rule.ID).Count(&active).Error; err != nil {
			return err
		}
		changed := existing.TargetType != rule.TargetType || existing.Metric != rule.Metric || existing.ScopeJSON != rule.ScopeJSON || existing.ConditionJSON != rule.ConditionJSON || existing.ActionJSON != rule.ActionJSON || existing.RecoveryJSON != rule.RecoveryJSON
		if active > 0 && changed {
			return errors.New("存在未恢复事件，不能修改规则判定或动作；请先处理事件")
		}
		rule.CreatedAt = existing.CreatedAt
		if err := tx.Model(&existing).Select("*").Omit("id", "created_at", "deleted_at").Updates(rule).Error; err != nil {
			return err
		}
		// 修改判定参数后从新证据重新累计，保留历史事件及执行记录。
		if changed {
			if err := tx.Model(&AlertRuleState{}).Where("rule_id = ?", rule.ID).Updates(map[string]any{"state": AlertStateNormal, "consecutive_violations": 0, "consecutive_recoveries": 0, "history_json": "[]", "baseline_value": nil, "last_observed_at": nil}).Error; err != nil {
				return err
			}
		}
		return replaceRuleChannels(tx, rule.ID, channelIDs)
	})
}

func (r *Alerts) DeleteRule(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing AlertRule
		if err := tx.First(&existing, id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&AlertEvent{}).Where("rule_id = ? AND recovered_at IS NULL", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("请先处理未恢复事件，再删除规则；删除不会自动启用任何目标")
		}
		if err := tx.Where("rule_id = ?", id).Delete(&AlertRuleNotificationChannel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", id).Delete(&AlertRuleState{}).Error; err != nil {
			return err
		}
		return tx.Delete(&AlertRule{}, id).Error
	})
}

func replaceRuleChannels(tx *gorm.DB, ruleID uint, channelIDs []uint) error {
	if err := tx.Where("rule_id = ?", ruleID).Delete(&AlertRuleNotificationChannel{}).Error; err != nil {
		return err
	}
	seen := make(map[uint]struct{}, len(channelIDs))
	rows := make([]AlertRuleNotificationChannel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID == 0 {
			return errors.New("通知渠道 ID 无效")
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		seen[channelID] = struct{}{}
		var channel NotificationChannel
		if err := tx.First(&channel, channelID).Error; err != nil {
			return fmt.Errorf("通知渠道 %d 不存在: %w", channelID, err)
		}
		rows = append(rows, AlertRuleNotificationChannel{RuleID: ruleID, NotificationChannelID: channelID})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

func (r *Alerts) GetOrCreateState(key AlertStateKey) (*AlertRuleState, error) {
	var state AlertRuleState
	err := r.db.Where("rule_id = ? AND target_type = ? AND relay_station_id = ? AND target_external_id = ?", key.RuleID, key.TargetType, key.RelayStationID, key.TargetExternalID).First(&state).Error
	if err == nil {
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	state = AlertRuleState{
		RuleID: key.RuleID, TargetType: key.TargetType, RelayStationID: key.RelayStationID,
		TargetExternalID: key.TargetExternalID, State: AlertStateNormal,
	}
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return nil, err
	}
	if err := r.db.Where("rule_id = ? AND target_type = ? AND relay_station_id = ? AND target_external_id = ?", key.RuleID, key.TargetType, key.RelayStationID, key.TargetExternalID).First(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *Alerts) SaveState(state *AlertRuleState) error {
	return r.db.Save(state).Error
}

func (r *Alerts) UpsertOpenEvent(event *AlertEvent) error {
	var current AlertEvent
	err := r.db.Where("rule_id = ? AND target_type = ? AND relay_station_id = ? AND target_external_id = ? AND recovered_at IS NULL", event.RuleID, event.TargetType, event.RelayStationID, event.TargetExternalID).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(event).Error
	}
	if err != nil {
		return err
	}
	event.ID = current.ID
	return r.db.Model(&current).Updates(map[string]any{
		"state": event.State, "actual_value": event.ActualValue, "threshold_value": event.ThresholdValue,
		"summary": event.Summary, "sample_json": event.SampleJSON, "action_summary_json": event.ActionSummaryJSON,
		"fired_at": event.FiredAt, "last_notified_at": event.LastNotifiedAt, "updated_at": event.UpdatedAt,
	}).Error
}

func (r *Alerts) RecoverEvent(id uint, at time.Time, summary string) error {
	return r.db.Model(&AlertEvent{}).Where("id = ? AND recovered_at IS NULL", id).Updates(map[string]any{
		"state": AlertStateRecovered, "recovered_at": at, "summary": summary, "updated_at": at,
	}).Error
}

func (r *Alerts) AcknowledgeEvent(id uint, at time.Time) error {
	return r.db.Model(&AlertEvent{}).Where("id = ?", id).Update("acknowledged_at", at).Error
}

type AlertSummary struct {
	OpenEvents      int64 `json:"open_events"`
	EnabledRules    int64 `json:"enabled_rules"`
	EnabledChannels int64 `json:"enabled_channels"`
	ActionsToday    int64 `json:"actions_today"`
}

func (r *Alerts) Summary(now time.Time) (AlertSummary, error) {
	var summary AlertSummary
	if err := r.db.Model(&AlertEvent{}).Where("recovered_at IS NULL").Count(&summary.OpenEvents).Error; err != nil {
		return summary, err
	}
	if err := r.db.Model(&AlertRule{}).Where("enabled = ?", true).Count(&summary.EnabledRules).Error; err != nil {
		return summary, err
	}
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(loc)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if err := r.db.Model(&AlertActionLog{}).Where("success = ? AND created_at >= ?", true, midnight).Count(&summary.ActionsToday).Error; err != nil {
		return summary, err
	}
	return summary, nil
}

func (r *Alerts) ListEvents(filter AlertEventFilter) ([]AlertEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := r.db.Model(&AlertEvent{}).Order("updated_at DESC, id DESC").Limit(limit).Offset(filter.Offset)
	if filter.From != nil {
		query = query.Where("started_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("started_at <= ?", *filter.To)
	}
	if filter.Severity != "" {
		query = query.Where("severity = ?", filter.Severity)
	}
	if filter.State == "open" {
		query = query.Where("recovered_at IS NULL")
	} else if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.TargetType != "" {
		query = query.Where("target_type = ?", filter.TargetType)
	}
	if filter.StationID != 0 {
		query = query.Where("relay_station_id = ?", filter.StationID)
	}
	list := make([]AlertEvent, 0)
	err := query.Find(&list).Error
	return list, err
}

func (r *Alerts) FindEvent(id uint) (*AlertEvent, error) {
	var event AlertEvent
	if err := r.db.First(&event, id).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *Alerts) FindOpenEvent(ruleID uint, targetType string, stationID uint, targetExternalID string) (*AlertEvent, error) {
	var event AlertEvent
	err := r.db.Where("rule_id = ? AND target_type = ? AND relay_station_id = ? AND target_external_id = ? AND recovered_at IS NULL", ruleID, targetType, stationID, targetExternalID).
		Order("started_at DESC, id DESC").First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *Alerts) ListActionLogs(eventID uint) ([]AlertActionLog, error) {
	list := make([]AlertActionLog, 0)
	err := r.db.Where("event_id = ?", eventID).Order("created_at ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *Alerts) SaveActionLog(logEntry *AlertActionLog) error {
	return r.db.Create(logEntry).Error
}

func (r *Alerts) SaveMetricSample(sample *AlertMetricSample) error {
	return r.db.Create(sample).Error
}

func (r *Alerts) LatestMetricSample(ruleID uint, targetType string, stationID uint, targetExternalID, metric string) (*AlertMetricSample, error) {
	var sample AlertMetricSample
	err := r.db.Where("rule_id = ? AND target_type = ? AND relay_station_id = ? AND target_external_id = ? AND metric = ?", ruleID, targetType, stationID, targetExternalID, metric).
		Order("observed_at DESC, id DESC").First(&sample).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sample, nil
}

func (r *Alerts) ListRuleChannelIDs(ruleID uint) ([]uint, error) {
	var rows []AlertRuleNotificationChannel
	if err := r.db.Where("rule_id = ?", ruleID).Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.NotificationChannelID)
	}
	return ids, nil
}

func (r *Alerts) UpdateActionLog(entry *AlertActionLog) error { return r.db.Save(entry).Error }
func (r *Alerts) SaveEvent(event *AlertEvent) error           { return r.db.Save(event).Error }

func (r *Alerts) PendingRecoveryNotifications() ([]AlertEvent, error) {
	list := make([]AlertEvent, 0)
	err := r.db.Where("recovery_notification_pending = ? AND recovered_at IS NOT NULL", true).Order("id").Find(&list).Error
	return list, err
}

func (r *Alerts) RecoveryNotificationSent(eventID uint) error {
	return r.db.Model(&AlertEvent{}).Where("id = ?", eventID).Update("recovery_notification_pending", false).Error
}

func (r *Alerts) ListTargetStates(targetType string, stationID uint, targetID string) ([]AlertRuleState, error) {
	var list []AlertRuleState
	err := r.db.Where("target_type = ? AND relay_station_id = ? AND target_external_id = ?", targetType, stationID, targetID).Find(&list).Error
	return list, err
}

func (r *Alerts) ListRuleStates(ruleID uint) ([]AlertRuleState, error) {
	var list []AlertRuleState
	err := r.db.Where("rule_id = ?", ruleID).Find(&list).Error
	return list, err
}

type AlertAutomationOwnership struct {
	EventID     uint   `json:"event_id"`
	Owned       bool   `json:"owned"`
	Hold        bool   `json:"hold"`
	Before      bool   `json:"before"`
	After       bool   `json:"after"`
	Fingerprint string `json:"fingerprint"`
}

func (r *Alerts) OtherAutomationHold(state AlertRuleState) (bool, error) {
	states, err := r.ListTargetStates(state.TargetType, state.RelayStationID, state.TargetExternalID)
	if err != nil {
		return true, err
	}
	for _, other := range states {
		if other.ID == state.ID || other.ManualIntervened {
			continue
		}
		var hold AlertAutomationOwnership
		if err := json.Unmarshal([]byte(other.AutomaticActionJSON), &hold); err != nil {
			return true, err
		}
		if hold.Hold {
			return true, nil
		}
	}
	return false, nil
}

func (r *Alerts) MarkManualIntervention(targetType string, stationID uint, targetID string) error {
	return r.db.Model(&AlertRuleState{}).Where("target_type = ? AND relay_station_id = ? AND target_external_id = ?", targetType, stationID, targetID).Update("manual_intervened", true).Error
}

func (r *Alerts) PruneSamples(before time.Time) error {
	return r.db.Where("observed_at < ?", before).Delete(&AlertMetricSample{}).Error
}

func (r *RelayStations) MarkAlertManualIntervention(targetType string, stationID uint, targetID string) error {
	return NewAlerts(r.db).MarkManualIntervention(targetType, stationID, targetID)
}

func (r *RelayStations) WithAlertMutationLock(ctx context.Context, fn func() error) error {
	return NewAlerts(r.db).WithMutationLock(ctx, func(_ *Alerts) error { return fn() })
}
