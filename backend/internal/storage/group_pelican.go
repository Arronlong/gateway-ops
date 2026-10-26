package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupPelicanConfig struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	RelayStationID  uint       `gorm:"not null;uniqueIndex:idx_group_pelican_target" json:"relay_station_id"`
	GroupExternalID int64      `gorm:"not null;uniqueIndex:idx_group_pelican_target" json:"group_external_id"`
	Enabled         bool       `gorm:"not null;default:false" json:"enabled"`
	CronExpression  string     `gorm:"size:128;not null" json:"-"`
	NextRunAt       time.Time  `gorm:"not null;index" json:"next_run_at"`
	LeaseUntil      *time.Time `json:"lease_until,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (GroupPelicanConfig) TableName() string { return "relay_group_pelican_configs" }

type GroupPelicanResult struct {
	ID                      uint      `gorm:"primaryKey" json:"id"`
	RelayStationID          uint      `gorm:"not null;index:idx_group_pelican_history,priority:1" json:"relay_station_id"`
	GroupExternalID         int64     `gorm:"not null;index:idx_group_pelican_history,priority:2" json:"group_external_id"`
	GroupName               string    `gorm:"size:256;not null" json:"group_name"`
	Model                   string    `gorm:"size:256;not null" json:"model"`
	Prompt                  string    `gorm:"type:text;not null" json:"prompt,omitempty"`
	ReasoningEffort         string    `gorm:"size:16;not null" json:"reasoning_effort"`
	Trigger                 string    `gorm:"size:16;not null" json:"trigger"`
	Status                  string    `gorm:"size:16;not null" json:"status"`
	HTML                    string    `gorm:"type:text;not null" json:"html,omitempty"`
	Error                   string    `gorm:"type:text;not null" json:"error"`
	DurationMS              int64     `gorm:"not null" json:"duration_ms"`
	StartedAt               time.Time `gorm:"not null" json:"started_at"`
	FinishedAt              time.Time `gorm:"not null;index:idx_group_pelican_history,priority:3,sort:desc" json:"finished_at"`
	UpstreamGroupName       string    `gorm:"size:256" json:"upstream_group_name"`
	UpstreamGroupMultiplier *float64  `json:"upstream_group_multiplier"`
}

func (GroupPelicanResult) TableName() string { return "relay_group_pelican_results" }
func (r GroupPelicanResult) Artifact() PelicanResult {
	return PelicanResult{ID: r.ID, RelayStationID: r.RelayStationID, GroupExternalID: r.GroupExternalID, AccountName: r.GroupName, Model: r.Model, Prompt: r.Prompt, ReasoningEffort: r.ReasoningEffort, Trigger: r.Trigger, Status: r.Status, HTML: r.HTML, Error: r.Error, DurationMS: r.DurationMS, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, UpstreamGroupName: r.UpstreamGroupName, UpstreamGroupMultiplier: r.UpstreamGroupMultiplier}
}

func (r *RelayStations) GroupPelicanConfig(stationID uint, groupID int64) (*GroupPelicanConfig, error) {
	var config GroupPelicanConfig
	err := r.db.Where("relay_station_id = ? AND group_external_id = ?", stationID, groupID).First(&config).Error
	return &config, err
}
func (r *RelayStations) SaveGroupPelicanConfig(config *GroupPelicanConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var station RelayStation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&station, config.RelayStationID).Error; err != nil {
			return err
		}
		var group RelayGroup
		if err := tx.Where("relay_station_id = ? AND external_id = ?", config.RelayStationID, config.GroupExternalID).First(&group).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "relay_station_id"}, {Name: "group_external_id"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "cron_expression", "next_run_at", "updated_at"})}).Create(config).Error
	})
}
func (r *RelayStations) ListGroupPelicanConfigs(stationID uint) ([]GroupPelicanConfig, error) {
	var configs []GroupPelicanConfig
	err := r.db.Table("relay_group_pelican_configs c").Select("c.*").Joins("JOIN relay_groups g ON g.relay_station_id=c.relay_station_id AND g.external_id=c.group_external_id").Where("c.relay_station_id = ?", stationID).Order("c.group_external_id ASC").Find(&configs).Error
	return configs, err
}
func (r *RelayStations) DueGroupPelicanConfigs(now time.Time) ([]GroupPelicanConfig, error) {
	var configs []GroupPelicanConfig
	err := r.db.Table("relay_group_pelican_configs c").Select("c.*").Joins("JOIN relay_stations s ON s.id=c.relay_station_id AND s.deleted_at IS NULL").Joins("JOIN relay_groups g ON g.relay_station_id=c.relay_station_id AND g.external_id=c.group_external_id").Where("c.enabled = ? AND c.next_run_at <= ? AND (c.lease_until IS NULL OR c.lease_until <= ?)", true, now, now).Order("c.next_run_at ASC").Limit(8).Find(&configs).Error
	return configs, err
}
func (r *RelayStations) ClaimGroupPelican(id uint, now, until, expected time.Time, scheduled bool) (bool, error) {
	query := r.db.Model(&GroupPelicanConfig{}).Where("id = ? AND (lease_until IS NULL OR lease_until <= ?)", id, now)
	if scheduled {
		query = query.Where("enabled = ? AND next_run_at = ?", true, expected)
	}
	result := query.Update("lease_until", until)
	return result.RowsAffected == 1, result.Error
}
func (r *RelayStations) FinishGroupPelican(id uint, until, expected, next time.Time) error {
	values := map[string]any{"lease_until": nil}
	if !next.IsZero() {
		values["next_run_at"] = gorm.Expr("CASE WHEN next_run_at = ? THEN ? ELSE next_run_at END", expected, next)
	}
	return r.db.Model(&GroupPelicanConfig{}).Where("id = ? AND lease_until = ?", id, until).Updates(values).Error
}
func (r *RelayStations) SaveGroupPelicanResult(result *GroupPelicanResult) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var station RelayStation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&station, result.RelayStationID).Error; err != nil {
			return err
		}
		if result.ID == 0 {
			if err := tx.Create(result).Error; err != nil {
				return err
			}
		} else {
			updated := tx.Model(&GroupPelicanResult{}).Where("id = ? AND relay_station_id = ? AND group_external_id = ?", result.ID, result.RelayStationID, result.GroupExternalID).Select("*").Omit("id").Updates(result)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		if result.Status == "running" {
			return nil
		}
		limit, err := NewSettings(tx).PelicanHistoryCount()
		if err != nil {
			return err
		}
		kept := tx.Model(&GroupPelicanResult{}).Select("id").Where("relay_station_id = ? AND group_external_id = ?", result.RelayStationID, result.GroupExternalID).Order("finished_at DESC, id DESC").Limit(limit)
		return tx.Where("relay_station_id = ? AND group_external_id = ? AND status <> 'running' AND id NOT IN (?)", result.RelayStationID, result.GroupExternalID, kept).Delete(&GroupPelicanResult{}).Error
	})
}
func (r *RelayStations) GroupPelicanGallery(stationID uint, limit int, publicOnly ...bool) ([]PelicanResult, error) {
	var rows []GroupPelicanResult
	visibility := ""
	if len(publicOnly) > 0 && publicOnly[0] {
		visibility = " AND groups.monitor_enabled = TRUE"
	}
	err := r.db.Raw(`SELECT id, relay_station_id, group_external_id, group_name, model, reasoning_effort, trigger, status, error, duration_ms, started_at, finished_at, upstream_group_name, upstream_group_multiplier FROM (
	 SELECT results.*, ROW_NUMBER() OVER(PARTITION BY results.group_external_id ORDER BY results.finished_at DESC,results.id DESC) AS row_number
	 FROM relay_group_pelican_results results JOIN relay_groups groups ON groups.relay_station_id=results.relay_station_id AND groups.external_id=results.group_external_id
	 WHERE results.relay_station_id = ?`+visibility+`
	) ranked WHERE row_number <= ? ORDER BY group_external_id,finished_at DESC,id DESC`, stationID, normalizedPelicanHistoryCount(limit)).Scan(&rows).Error
	results := make([]PelicanResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.Artifact())
	}
	return results, err
}
func (r *RelayStations) GroupPelicanResult(stationID uint, groupID int64, id uint) (*PelicanResult, error) {
	var result GroupPelicanResult
	err := r.db.Where("relay_station_id = ? AND group_external_id = ? AND id = ?", stationID, groupID, id).First(&result).Error
	artifact := result.Artifact()
	return &artifact, err
}
func (r *RelayStations) DeleteGroupPelicanResult(stationID uint, groupID int64, id uint) error {
	result := r.db.Where("relay_station_id = ? AND group_external_id = ? AND id = ?", stationID, groupID, id).Delete(&GroupPelicanResult{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *RelayStations) PublicPelicanGroup(stationID uint, groupID int64) error {
	group, err := r.FindGroupByExternalID(stationID, groupID)
	if err != nil {
		return err
	}
	if !group.MonitorEnabled {
		return gorm.ErrRecordNotFound
	}
	if _, err := r.FindByID(stationID); err != nil {
		return err
	}
	return nil
}

func (r *RelayStations) PruneGroupPelicanResults(limit int) error {
	if limit < 1 || limit > PelicanHistoryLimitMax {
		return errors.New("invalid pelican history count")
	}
	return r.db.Exec(`DELETE FROM relay_group_pelican_results WHERE id IN (SELECT id FROM (SELECT id,ROW_NUMBER() OVER(PARTITION BY relay_station_id,group_external_id ORDER BY finished_at DESC,id DESC) AS row_number FROM relay_group_pelican_results) ranked WHERE row_number > ?)`, limit).Error
}
