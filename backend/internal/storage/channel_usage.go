package storage

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelUsage stores request history collected from each monitored channel's
// own usage/log API. It never reads relay station tables.
type ChannelUsage struct{ db *gorm.DB }

func NewChannelUsage(db *gorm.DB) *ChannelUsage { return &ChannelUsage{db: db} }

type channelUsageModelRow struct {
	ChannelID uint
	Model     string
}

// DistinctModelsByChannels returns models observed in each channel's own usage
// history. It uses one grouped query so channel list search does not fan out.
func (r *ChannelUsage) DistinctModelsByChannels(channelIDs []uint) (map[uint][]string, error) {
	result := make(map[uint][]string, len(channelIDs))
	if len(channelIDs) == 0 {
		return result, nil
	}

	var rows []channelUsageModelRow
	if err := r.db.Model(&ChannelUsageEvent{}).
		Select("channel_id, model").
		Where("channel_id IN ? AND model <> ''", channelIDs).
		Group("channel_id, model").
		Order("channel_id ASC, model ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return groupChannelUsageModels(rows), nil
}

func groupChannelUsageModels(rows []channelUsageModelRow) map[uint][]string {
	result := make(map[uint][]string)
	for _, row := range rows {
		result[row.ChannelID] = append(result[row.ChannelID], row.Model)
	}
	return result
}

func (r *ChannelUsage) Upsert(events []ChannelUsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}, {Name: "channel_account_id"}, {Name: "source"}, {Name: "remote_usage_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"request_id", "model", "first_token_ms", "duration_ms", "success_known", "success",
			"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens",
			"actual_cost", "total_cost", "occurred_at", "synced_at",
		}),
	}).CreateInBatches(events, 200).Error
}

func (r *ChannelUsage) LatestOccurredAt(channelID, channelAccountID uint) (*time.Time, error) {
	var latest *time.Time
	err := r.db.Model(&ChannelUsageEvent{}).
		Where("channel_id = ? AND channel_account_id = ?", channelID, channelAccountID).
		Select("MAX(occurred_at)").Scan(&latest).Error
	return latest, err
}

func (r *ChannelUsage) SyncState(channelID, channelAccountID uint) (*ChannelUsageSyncState, error) {
	var state ChannelUsageSyncState
	err := r.db.Where("channel_id = ? AND channel_account_id = ?", channelID, channelAccountID).First(&state).Error
	if err == gorm.ErrRecordNotFound {
		return &ChannelUsageSyncState{ChannelID: channelID, ChannelAccountID: channelAccountID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *ChannelUsage) SaveSyncState(state *ChannelUsageSyncState) error {
	if state == nil {
		return nil
	}
	state.UpdatedAt = time.Now()
	return r.db.Save(state).Error
}

func (r *ChannelUsage) RecentSamples(channelID uint, limit int) ([]ChannelUsageSample, error) {
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	var events []ChannelUsageEvent
	if err := r.db.Where("channel_id = ?", channelID).
		Order("occurred_at DESC").Order("id DESC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	return channelUsageSamples(events), nil
}

func (r *ChannelUsage) SamplesBetween(channelID uint, since, until time.Time, limit int) ([]ChannelUsageSample, error) {
	q := r.db.Where("channel_id = ? AND occurred_at >= ? AND occurred_at < ?", channelID, since, until).
		Order("occurred_at DESC").Order("id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var events []ChannelUsageEvent
	if err := q.Find(&events).Error; err != nil {
		return nil, err
	}
	return channelUsageSamples(events), nil
}

func (r *ChannelUsage) EventsBetween(channelID uint, since, until time.Time) ([]ChannelUsageEvent, error) {
	var events []ChannelUsageEvent
	err := r.db.Where("channel_id = ? AND occurred_at >= ? AND occurred_at < ?", channelID, since, until).
		Order("occurred_at ASC").Order("id ASC").Find(&events).Error
	return events, err
}

func (r *ChannelUsage) CostTotals(since time.Time) (map[uint]float64, map[uint]int64, error) {
	type row struct {
		ChannelID uint
		Amount    float64
		Requests  int64
	}
	var rows []row
	q := r.db.Model(&ChannelUsageEvent{}).
		Select("channel_id, COALESCE(SUM(actual_cost), 0) AS amount, COUNT(*) AS requests").
		Group("channel_id")
	if !since.IsZero() {
		q = q.Where("occurred_at >= ?", since)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	amounts := make(map[uint]float64, len(rows))
	requests := make(map[uint]int64, len(rows))
	for _, item := range rows {
		amounts[item.ChannelID] = item.Amount
		requests[item.ChannelID] = item.Requests
	}
	return amounts, requests, nil
}

func channelUsageSamples(events []ChannelUsageEvent) []ChannelUsageSample {
	out := make([]ChannelUsageSample, 0, len(events))
	for _, event := range events {
		out = append(out, ChannelUsageSample{
			FirstTokenMS:        event.FirstTokenMS,
			DurationMS:          event.DurationMS,
			SuccessKnown:        event.SuccessKnown,
			Success:             event.Success,
			CreatedAt:           event.OccurredAt,
			Model:               event.Model,
			InputTokens:         event.InputTokens,
			OutputTokens:        event.OutputTokens,
			CacheReadTokens:     event.CacheReadTokens,
			CacheCreationTokens: event.CacheCreationTokens,
		})
	}
	return out
}
