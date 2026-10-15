package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Alerts struct{ db *gorm.DB }

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
	var list []AlertRule
	return list, query.Find(&list).Error
}

func (r *Alerts) FindRule(id uint) (*AlertRule, error) {
	var rule AlertRule
	if err := r.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
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
		if err := tx.Save(rule).Error; err != nil {
			return err
		}
		return replaceRuleChannels(tx, rule.ID, channelIDs)
	})
}

func (r *Alerts) DeleteRule(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
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
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		seen[channelID] = struct{}{}
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

func (r *Alerts) ListEvents(filter AlertEventFilter) ([]AlertEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := r.db.Model(&AlertEvent{}).Order("updated_at DESC").Limit(limit)
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.TargetType != "" {
		query = query.Where("target_type = ?", filter.TargetType)
	}
	if filter.StationID != 0 {
		query = query.Where("relay_station_id = ?", filter.StationID)
	}
	var list []AlertEvent
	return list, query.Find(&list).Error
}

func (r *Alerts) FindEvent(id uint) (*AlertEvent, error) {
	var event AlertEvent
	if err := r.db.First(&event, id).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *Alerts) ListActionLogs(eventID uint) ([]AlertActionLog, error) {
	var list []AlertActionLog
	err := r.db.Where("event_id = ?", eventID).Order("created_at ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *Alerts) SaveActionLog(logEntry *AlertActionLog) error {
	return r.db.Create(logEntry).Error
}

func (r *Alerts) SaveMetricSample(sample *AlertMetricSample) error {
	return r.db.Create(sample).Error
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
