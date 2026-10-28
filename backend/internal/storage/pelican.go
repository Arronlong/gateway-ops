package storage

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

const (
	PelicanHistoryLimit    = 30
	PelicanHistoryLimitMax = 100
)

func normalizedPelicanHistoryCount(count int) int {
	if count < 1 || count > PelicanHistoryLimitMax {
		return PelicanHistoryLimit
	}
	return count
}

type PelicanConfig struct {
	ChannelID         uint       `gorm:"not null;default:0" json:"-"`
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
	GroupExternalID         int64     `gorm:"-" json:"group_external_id,omitempty"`
	ID                      uint      `gorm:"primaryKey" json:"id"`
	RelayStationID          uint      `gorm:"not null;index:idx_pelican_history,priority:1" json:"relay_station_id"`
	AccountExternalID       int64     `gorm:"not null;index:idx_pelican_history,priority:2" json:"account_external_id"`
	AccountName             string    `gorm:"size:256;not null" json:"account_name"`
	Model                   string    `gorm:"size:256;not null" json:"model"`
	Prompt                  string    `gorm:"type:text;not null" json:"prompt,omitempty"`
	ReasoningEffort         string    `gorm:"size:16;not null" json:"reasoning_effort"`
	Trigger                 string    `gorm:"size:16;not null" json:"trigger"`
	Status                  string    `gorm:"size:16;not null" json:"status"`
	HTML                    string    `gorm:"type:text;not null" json:"html,omitempty"`
	Error                   string    `gorm:"type:text;not null" json:"error"`
	DurationMS              int64     `gorm:"not null" json:"duration_ms"`
	StartedAt               time.Time `gorm:"not null" json:"started_at"`
	FinishedAt              time.Time `gorm:"not null;index:idx_pelican_history,priority:3,sort:desc" json:"finished_at"`
	UpstreamGroupName       string    `gorm:"size:256;not null;default:''" json:"upstream_group_name"`
	UpstreamGroupMultiplier *float64  `json:"upstream_group_multiplier"`
}

type PelicanLibraryItem struct {
	PelicanResult
	StationName string `json:"station_name"`
}

type PelicanLibraryFilter struct {
	StationID    uint
	Status       string
	Search       string
	Page         int
	PageSize     int
	HistoryCount int
}

type PelicanLibraryPage struct {
	Results  []PelicanLibraryItem `json:"results"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
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
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "relay_station_id"}, {Name: "account_external_id"}}, DoUpdates: clause.AssignmentColumns([]string{"channel_id", "enabled", "model", "prompt", "reasoning_effort", "parallel_count", "cron_expression", "next_run_at", "updated_at"})}).Create(config).Error
	})
}
func (r *RelayStations) ListPelicanConfigs(stationID uint) ([]PelicanConfig, error) {
	rows := make([]PelicanConfig, 0)
	err := r.db.Table("relay_pelican_configs c").Select("c.*").Joins("JOIN relay_accounts a ON a.relay_station_id = c.relay_station_id AND a.external_id = c.account_external_id").Where("c.relay_station_id = ?", stationID).Order("c.account_external_id ASC").Find(&rows).Error
	return rows, err
}
func (r *RelayStations) DuePelicanConfigs(now time.Time) ([]PelicanConfig, error) {
	rows := make([]PelicanConfig, 0)
	err := r.db.Table("relay_pelican_configs c").Select("c.*").Joins("JOIN relay_stations s ON s.id = c.relay_station_id AND s.deleted_at IS NULL").Joins("JOIN relay_accounts a ON a.relay_station_id = c.relay_station_id AND a.external_id = c.account_external_id").Where("c.enabled = ? AND c.next_run_at <= ? AND (c.lease_until IS NULL OR c.lease_until <= ?)", true, now, now).Order("c.next_run_at ASC").Limit(8).Find(&rows).Error
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
		if result.ID == 0 {
			if err := tx.Create(result).Error; err != nil {
				return err
			}
		} else {
			updated := tx.Model(&PelicanResult{}).Where("id = ? AND relay_station_id = ? AND account_external_id = ?", result.ID, result.RelayStationID, result.AccountExternalID).Select("*").Omit("id").Updates(result)
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
		historyCount := PelicanHistoryLimit
		var setting AppSetting
		if err := tx.First(&setting, "key = ?", SettingPelicanHistoryCount).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			parsed, err := strconv.Atoi(strings.TrimSpace(setting.Value))
			if err != nil || parsed < 1 || parsed > PelicanHistoryLimitMax {
				return errors.New("invalid pelican history count")
			}
			historyCount = parsed
		}
		kept := tx.Model(&PelicanResult{}).Select("id").Where("relay_station_id = ? AND account_external_id = ?", result.RelayStationID, result.AccountExternalID).Order("finished_at DESC, id DESC").Limit(historyCount)
		return tx.Where("relay_station_id = ? AND account_external_id = ? AND status <> 'running' AND id NOT IN (?)", result.RelayStationID, result.AccountExternalID, kept).Delete(&PelicanResult{}).Error
	})
}
func (r *RelayStations) PelicanGallery(stationID uint, historyCounts ...int) ([]PelicanResult, error) {
	rows := make([]PelicanResult, 0)
	historyCount := PelicanHistoryLimit
	if len(historyCounts) > 0 {
		historyCount = normalizedPelicanHistoryCount(historyCounts[0])
	}
	err := r.db.Raw(`SELECT id, relay_station_id, account_external_id, account_name, model, reasoning_effort, trigger, status, error, duration_ms, started_at, finished_at, upstream_group_name, upstream_group_multiplier FROM (
 SELECT results.*, ROW_NUMBER() OVER (PARTITION BY results.account_external_id ORDER BY results.finished_at DESC,results.id DESC) AS row_number
 FROM relay_pelican_results results JOIN relay_accounts accounts ON accounts.relay_station_id = results.relay_station_id AND accounts.external_id = results.account_external_id WHERE results.relay_station_id = ?
	) ranked WHERE row_number <= ? ORDER BY account_external_id, finished_at DESC,id DESC`, stationID, historyCount).Scan(&rows).Error
	return rows, err
}

func (r *RelayStations) PelicanLibrary(filter PelicanLibraryFilter) (PelicanLibraryPage, error) {
	page := PelicanLibraryPage{Page: filter.Page, PageSize: filter.PageSize}
	if page.Page < 1 {
		page.Page = 1
	}
	if page.PageSize < 1 {
		page.PageSize = 60
	}
	if page.PageSize > 100 {
		page.PageSize = 100
	}
	historyCount := normalizedPelicanHistoryCount(filter.HistoryCount)
	status := strings.TrimSpace(filter.Status)
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "success" && status != "failed" {
		return page, errors.New("invalid pelican result status")
	}
	search := strings.TrimSpace(filter.Search)
	if len(search) > 100 {
		return page, errors.New("pelican result search is too long")
	}
	base := func() *gorm.DB {
		return r.db.Table("relay_pelican_results AS results").
			Select("results.id, results.relay_station_id, results.account_external_id, results.account_name, results.model, results.reasoning_effort, results.trigger, results.status, results.error, results.duration_ms, results.started_at, results.finished_at, results.upstream_group_name, results.upstream_group_multiplier, stations.name AS station_name, ROW_NUMBER() OVER (PARTITION BY results.relay_station_id, results.account_external_id ORDER BY results.finished_at DESC, results.id DESC) AS row_number").
			Joins("JOIN relay_stations AS stations ON stations.id = results.relay_station_id AND stations.deleted_at IS NULL").
			Joins("JOIN relay_accounts AS accounts ON accounts.relay_station_id = results.relay_station_id AND accounts.external_id = results.account_external_id").
			Where("(stations.type = ? OR stations.type = '')", "sub2api")
	}
	filtered := func() *gorm.DB {
		query := r.db.Table("(?) AS recent", base()).Where("recent.row_number <= ?", historyCount)
		if filter.StationID > 0 {
			query = query.Where("recent.relay_station_id = ?", filter.StationID)
		}
		if status != "all" {
			query = query.Where("recent.status = ?", status)
		}
		if search != "" {
			pattern := "%" + search + "%"
			query = query.Where("(recent.station_name ILIKE ? OR recent.account_name ILIKE ? OR recent.model ILIKE ?)", pattern, pattern, pattern)
		}
		return query
	}
	if err := filtered().Count(&page.Total).Error; err != nil {
		return page, err
	}
	err := filtered().Select("recent.id, recent.relay_station_id, recent.account_external_id, recent.account_name, recent.model, recent.reasoning_effort, recent.trigger, recent.status, recent.error, recent.duration_ms, recent.started_at, recent.finished_at, recent.upstream_group_name, recent.upstream_group_multiplier, recent.station_name").
		Order("recent.finished_at DESC, recent.id DESC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&page.Results).Error
	return page, err
}

func (r *RelayStations) PelicanResult(stationID uint, accountID int64, resultID uint) (*PelicanResult, error) {
	var result PelicanResult
	err := r.db.Where("relay_station_id = ? AND account_external_id = ? AND id = ?", stationID, accountID, resultID).First(&result).Error
	return &result, err
}

func (r *RelayStations) PoolPelicanGallery(stationID uint, limit int) ([]PelicanResult, error) {
	var results []PelicanResult
	err := r.db.Raw(`SELECT id, relay_station_id, account_external_id, account_name, model, reasoning_effort, trigger, status, error, duration_ms, started_at, finished_at, upstream_group_name, upstream_group_multiplier FROM (
	 SELECT results.*,ROW_NUMBER() OVER(PARTITION BY results.account_external_id ORDER BY results.finished_at DESC,results.id DESC) AS row_number
	 FROM relay_pelican_results results JOIN relay_accounts accounts ON accounts.relay_station_id=results.relay_station_id AND accounts.external_id=results.account_external_id
	 WHERE results.relay_station_id=? AND accounts.type=?
	) ranked WHERE row_number<=? ORDER BY account_external_id,finished_at DESC,id DESC`, stationID, "oauth", normalizedPelicanHistoryCount(limit)).Scan(&results).Error
	return results, err
}

func (r *RelayStations) DeletePelicanResult(stationID uint, accountID int64, resultID uint) error {
	result := r.db.Where("relay_station_id = ? AND account_external_id = ? AND id = ?", stationID, accountID, resultID).Delete(&PelicanResult{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
