package storage

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 每页数据、已见标识和连续断点必须在同一事务中提交。
func (r *ChannelUsage) CommitHistoryPage(state *ChannelUsageSyncState, runID, source string, events []ChannelUsageEvent, pageIdentities []string, since, until time.Time, done bool) error {
	if len(events) != len(pageIdentities) {
		return fmt.Errorf("usage page identities do not match its records")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		identities := make([]string, 0, len(events))
		for _, event := range events {
			if event.ChannelID != state.ChannelID || event.ChannelAccountID != state.ChannelAccountID || event.Source != source {
				return fmt.Errorf("usage page belongs to another account")
			}
			identities = append(identities, event.RemoteUsageID)
		}
		type occurrence struct {
			Identity string
			N        int
		}
		var previous []occurrence
		if len(identities) > 0 {
			if err := tx.Model(&ChannelUsageHistorySeen{}).Select("identity, MAX(occurrence) AS n").Where("run_id = ? AND identity IN ?", runID, identities).Group("identity").Scan(&previous).Error; err != nil {
				return err
			}
		}
		counts := make(map[string]int, len(previous))
		for _, item := range previous {
			counts[item.Identity] = item.N
		}
		var previousRows []ChannelUsageHistorySeen
		if len(pageIdentities) > 0 {
			if err := tx.Where("run_id = ? AND page_identity IN ?", runID, pageIdentities).Find(&previousRows).Error; err != nil {
				return err
			}
		}
		savedRows := make(map[string]ChannelUsageHistorySeen, len(previousRows))
		for _, row := range previousRows {
			savedRows[row.PageIdentity] = row
		}
		seen := make([]ChannelUsageHistorySeen, 0, len(events))
		persisted := make([]ChannelUsageEvent, 0, len(events))
		positions := make(map[string]int, len(events))
		for i := range events {
			identity := events[i].RemoteUsageID
			if saved, ok := savedRows[pageIdentities[i]]; ok {
				events[i].RemoteUsageID = saved.RemoteUsageID
			} else {
				counts[identity]++
				if source == string(ChannelTypeNewAPI) && counts[identity] > 1 {
					events[i].RemoteUsageID = fmt.Sprintf("%s:%d", identity, counts[identity])
				}
				row := ChannelUsageHistorySeen{RunID: runID, RemoteUsageID: events[i].RemoteUsageID, Identity: identity, Occurrence: counts[identity], PageIdentity: pageIdentities[i]}
				seen = append(seen, row)
				savedRows[pageIdentities[i]] = row
			}
			if position, ok := positions[events[i].RemoteUsageID]; ok {
				persisted[position] = events[i]
			} else {
				positions[events[i].RemoteUsageID] = len(persisted)
				persisted = append(persisted, events[i])
			}
		}
		if len(seen) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(seen, 200).Error; err != nil {
				return err
			}
		}
		repository := NewChannelUsage(tx)
		if err := repository.Upsert(persisted); err != nil {
			return err
		}
		if since.Before(until) {
			saved := tx.Model(&ChannelUsageHistorySeen{}).Select("remote_usage_id").Where("run_id = ?", runID)
			if err := tx.Where("channel_id = ? AND channel_account_id = ? AND source = ? AND occurred_at >= ? AND occurred_at < ?", state.ChannelID, state.ChannelAccountID, source, since, until).Where("remote_usage_id NOT IN (?)", saved).Delete(&ChannelUsageEvent{}).Error; err != nil {
				return err
			}
		}
		if done {
			if err := tx.Where("run_id = ?", runID).Delete(&ChannelUsageHistorySeen{}).Error; err != nil {
				return err
			}
		}
		return repository.SaveSyncState(state)
	})
}

func (r *ChannelUsage) HistoryPageOverlaps(runID string, identities []string) (bool, error) {
	seen := make(map[string]bool, len(identities))
	for _, identity := range identities {
		if seen[identity] {
			return true, nil
		}
		seen[identity] = true
	}
	if len(identities) == 0 {
		return false, nil
	}
	var n int64
	err := r.db.Model(&ChannelUsageHistorySeen{}).Where("run_id = ? AND page_identity IN ?", runID, identities).Count(&n).Error
	return n > 0, err
}

func (r *ChannelUsage) DiscardHistoryRun(state *ChannelUsageSyncState, runID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("run_id = ?", runID).Delete(&ChannelUsageHistorySeen{}).Error; err != nil {
			return err
		}
		state.HistoryResume = ""
		return NewChannelUsage(tx).SaveSyncState(state)
	})
}

func (r *ChannelUsage) ClearHistorySeen(runID string) error {
	return r.db.Where("run_id = ?", runID).Delete(&ChannelUsageHistorySeen{}).Error
}
