package storage

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

const PelicanHistoryLimit = 30

type PelicanConfig struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	RelayStationID    uint       `gorm:"not null;uniqueIndex:idx_pelican_account" json:"relay_station_id"`
	AccountExternalID int64      `gorm:"not null;uniqueIndex:idx_pelican_account" json:"account_external_id"`
	Enabled           bool       `gorm:"not null;default:false" json:"enabled"`
	Model             string     `gorm:"size:256;not null" json:"-"`
	Prompt            string     `gorm:"type:text;not null" json:"-"`
	ReasoningEffort   string     `gorm:"size:16;not null" json:"-"`
	ParallelCount     int        `gorm:"not null;default:1" json:"-"`
	CronExpression    string     `gorm:"size:128;not null" json:"-"`
	NextRunAt         time.Time  `gorm:"not null;index" json:"next_run_at"`
	LeaseUntil        *time.Time `json:"lease_until,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (PelicanConfig) TableName() string { return "relay_pelican_configs" }

type PelicanResult struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	RelayStationID    uint      `gorm:"not null;index:idx_pelican_history,priority:1" json:"relay_station_id"`
	AccountExternalID int64     `gorm:"not null;index:idx_pelican_history,priority:2" json:"account_external_id"`
	AccountName       string    `gorm:"size:256;not null" json:"account_name"`
	Model             string    `gorm:"size:256;not null" json:"model"`
	Prompt            string    `gorm:"type:text;not null" json:"prompt,omitempty"`
	ReasoningEffort   string    `gorm:"size:16;not null" json:"reasoning_effort"`
	Trigger           string    `gorm:"size:16;not null" json:"trigger"`
	Status            string    `gorm:"size:16;not null" json:"status"`
	HTML              string    `gorm:"type:text;not null" json:"html,omitempty"`
	Error             string    `gorm:"type:text;not null" json:"error"`
	DurationMS        int64     `gorm:"not null" json:"duration_ms"`
	StartedAt         time.Time `gorm:"not null" json:"started_at"`
	FinishedAt        time.Time `gorm:"not null;index:idx_pelican_history,priority:3,sort:desc" json:"finished_at"`
}

func (PelicanResult) TableName() string { return "relay_pelican_results" }

func (r *RelayStations) PelicanConfig(stationID uint, accountID int64) (*PelicanConfig, error) {
	var config PelicanConfig
	err := r.db.Where("relay_station_id = ? AND account_external_id = ?", stationID, accountID).First(&config).Error
	return &config, err
}
func (r *RelayStations) SavePelicanConfig(config *PelicanConfig) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var station RelayStation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&station, config.RelayStationID).Error; err != nil {
			return err
		}
		var account RelayAccount
		if err := tx.Where("relay_station_id = ? AND external_id = ?", config.RelayStationID, config.AccountExternalID).First(&account).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "relay_station_id"}, {Name: "account_external_id"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "model", "prompt", "reasoning_effort", "parallel_count", "cron_expression", "next_run_at", "updated_at"})}).Create(config).Error
	})
}
func (r *RelayStations) ListPelicanConfigs(stationID uint) ([]PelicanConfig, error) {
	rows := make([]PelicanConfig, 0)
	err := r.db.Table("relay_pelican_configs c").Select("c.*").Joins("JOIN relay_accounts a ON a.relay_station_id = c.relay_station_id AND a.external_id = c.account_external_id").Where("c.relay_station_id = ?", stationID).Order("c.account_external_id ASC").Find(&rows).Error
	return rows, err
}
func (r *RelayStations) DuePelicanConfigs(now time.Time) ([]PelicanConfig, error) {
	rows := make([]PelicanConfig, 0)
	err := r.db.Table("relay_pelican_configs c").Select("c.*").Joins("JOIN relay_stations s ON s.id = c.relay_station_id AND s.deleted_at IS NULL").Joins("JOIN relay_accounts a ON a.relay_station_id = c.relay_station_id AND a.external_id = c.account_external_id").Where("(s.type = ? OR s.type = '') AND c.enabled = ? AND c.next_run_at <= ? AND (c.lease_until IS NULL OR c.lease_until <= ?)", "sub2api", true, now, now).Order("c.next_run_at ASC").Limit(8).Find(&rows).Error
	return rows, err
}
func (r *RelayStations) ClaimPelican(configID uint, now, until, expected time.Time, scheduled bool) (bool, error) {
	query := r.db.Model(&PelicanConfig{}).Where("id = ? AND (lease_until IS NULL OR lease_until <= ?)", configID, now)
	updates := map[string]any{"lease_until": until}
	if scheduled {
		query = query.Where("enabled = ? AND next_run_at <= ? AND next_run_at = ?", true, now, expected)
	}
	result := query.Updates(updates)
	return result.RowsAffected == 1, result.Error
}
func (r *RelayStations) CompletePelican(configID uint, until, expected, next time.Time) error {
	return r.db.Model(&PelicanConfig{}).Where("id = ? AND lease_until = ?", configID, until).Updates(map[string]any{"lease_until": nil, "next_run_at": gorm.Expr("CASE WHEN next_run_at = ? THEN ? ELSE next_run_at END", expected, next)}).Error
}
func (r *RelayStations) FinishPelican(configID uint, until time.Time) error {
	return r.db.Model(&PelicanConfig{}).Where("id = ? AND lease_until = ?", configID, until).Update("lease_until", nil).Error
}
func (r *RelayStations) SavePelicanResult(result *PelicanResult) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var station RelayStation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&station, result.RelayStationID).Error; err != nil {
			return err
		}
		if err := tx.Create(result).Error; err != nil {
			return err
		}
		kept := tx.Model(&PelicanResult{}).Select("id").Where("relay_station_id = ? AND account_external_id = ?", result.RelayStationID, result.AccountExternalID).Order("finished_at DESC, id DESC").Limit(PelicanHistoryLimit)
		return tx.Where("relay_station_id = ? AND account_external_id = ? AND id NOT IN (?)", result.RelayStationID, result.AccountExternalID, kept).Delete(&PelicanResult{}).Error
	})
}
func (r *RelayStations) PelicanGallery(stationID uint) ([]PelicanResult, error) {
	rows := make([]PelicanResult, 0)
	err := r.db.Raw(`SELECT id, relay_station_id, account_external_id, account_name, model, reasoning_effort, trigger, status, error, duration_ms, started_at, finished_at FROM (
 SELECT results.*, ROW_NUMBER() OVER (PARTITION BY results.account_external_id ORDER BY results.finished_at DESC,results.id DESC) AS row_number
 FROM relay_pelican_results results JOIN relay_accounts accounts ON accounts.relay_station_id = results.relay_station_id AND accounts.external_id = results.account_external_id WHERE results.relay_station_id = ?
 ) ranked WHERE row_number <= ? ORDER BY account_external_id, finished_at DESC,id DESC`, stationID, PelicanHistoryLimit).Scan(&rows).Error
	return rows, err
}
func (r *RelayStations) PelicanResult(stationID uint, accountID int64, resultID uint) (*PelicanResult, error) {
	var result PelicanResult
	err := r.db.Where("relay_station_id = ? AND account_external_id = ? AND id = ?", stationID, accountID, resultID).First(&result).Error
	return &result, err
}
