package storage

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const relayUsageRetention = 35 * 24 * time.Hour

func (r *RelayStations) UsageSyncState(stationID uint) (RelayUsageSyncState, error) {
	var state RelayUsageSyncState
	err := r.db.First(&state, "relay_station_id = ?", stationID).Error
	if err == gorm.ErrRecordNotFound {
		return RelayUsageSyncState{RelayStationID: stationID}, nil
	}
	return state, err
}

func (r *RelayStations) SaveUsageSyncState(state RelayUsageSyncState) error {
	state.UpdatedAt = time.Now().UTC()
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "relay_station_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"forward_synced_at", "backfilled_at", "updated_at"}),
	}).Create(&state).Error
}

func (r *RelayStations) EarliestUsageOccurredAt(stationID uint) (time.Time, bool, error) {
	var event RelayUsageEvent
	err := r.db.Where("relay_station_id = ?", stationID).Order("occurred_at ASC, remote_usage_id ASC").First(&event).Error
	if err == gorm.ErrRecordNotFound {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return event.OccurredAt, true, nil
}

func (r *RelayStations) LatestUsageOccurredAt(stationID uint) (time.Time, bool, error) {
	var event RelayUsageEvent
	err := r.db.Where("relay_station_id = ?", stationID).Order("occurred_at DESC, remote_usage_id DESC").First(&event).Error
	if err == gorm.ErrRecordNotFound {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return event.OccurredAt, true, nil
}

func (r *RelayStations) UpsertUsageEvents(events []RelayUsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "relay_station_id"}, {Name: "remote_usage_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"account_external_id", "group_external_id", "group_name", "user_email",
			"model", "request_type", "first_token_ms", "duration_ms", "status_code",
			"cache_known", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens",
			"cache_creation5m_tokens", "cache_creation1h_tokens", "actual_cost", "total_cost",
			"account_rate_multiplier", "account_stats_cost", "occurred_at", "synced_at",
		}),
	}).CreateInBatches(events, 500).Error
}

func (r *RelayStations) ListUsageEvents(stationID uint, since, until time.Time) ([]RelayUsageEvent, error) {
	query := r.db.Where("relay_station_id = ?", stationID)
	if !since.IsZero() {
		query = query.Where("occurred_at >= ?", since)
	}
	if !until.IsZero() {
		query = query.Where("occurred_at < ?", until)
	}
	var events []RelayUsageEvent
	err := query.Order("occurred_at ASC, remote_usage_id ASC").Find(&events).Error
	return events, err
}

func (r *RelayStations) RecentUsageEventsByAccount(stationID uint, limit int) ([]RelayUsageEvent, error) {
	if limit <= 0 {
		limit = 60
	}
	var events []RelayUsageEvent
	err := r.db.Raw(`
		SELECT *
		FROM (
			SELECT relay_usage_events.*,
			       ROW_NUMBER() OVER (
			         PARTITION BY account_external_id
			         ORDER BY occurred_at DESC, remote_usage_id DESC
			       ) AS row_number
			FROM relay_usage_events
			WHERE relay_station_id = ?
		) ranked
		WHERE row_number <= ?
		ORDER BY account_external_id ASC, occurred_at DESC, remote_usage_id DESC
	`, stationID, limit).Scan(&events).Error
	return events, err
}

func (r *RelayStations) DeleteOldUsageEvents(stationID uint, cutoff time.Time) error {
	if cutoff.IsZero() {
		cutoff = time.Now().UTC().Add(-relayUsageRetention)
	}
	return r.db.Where("relay_station_id = ? AND occurred_at < ?", stationID, cutoff).Delete(&RelayUsageEvent{}).Error
}
