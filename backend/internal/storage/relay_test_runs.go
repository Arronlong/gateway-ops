package storage

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const RelayTestHistoryLimit = 30

type RelayTestCall struct {
	Index      int    `json:"index"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code"`
	DurationMS int64  `json:"duration_ms"`
	Output     string `json:"output"`
}
type RelayTestRun struct {
	ID             uint            `gorm:"primaryKey" json:"id"`
	RelayStationID uint            `gorm:"not null;index:idx_relay_test_station_time,priority:1" json:"relay_station_id"`
	StationName    string          `gorm:"size:256;not null" json:"station_name"`
	TargetType     string          `gorm:"size:16;not null" json:"target_type"`
	TargetID       int64           `gorm:"not null" json:"target_id"`
	TargetName     string          `gorm:"size:256;not null" json:"target_name"`
	Model          string          `gorm:"size:256;not null" json:"model"`
	Mode           string          `gorm:"size:16;not null" json:"mode"`
	Requested      int             `gorm:"not null" json:"requested"`
	Succeeded      int             `gorm:"not null" json:"succeeded"`
	Failed         int             `gorm:"not null" json:"failed"`
	DurationMS     int64           `gorm:"not null" json:"duration_ms"`
	SetupError     bool            `gorm:"not null;default:false" json:"setup_error"`
	Error          string          `gorm:"type:text;not null" json:"error"`
	StartedAt      time.Time       `gorm:"not null" json:"started_at"`
	FinishedAt     time.Time       `gorm:"not null;index:idx_relay_test_station_time,priority:2" json:"finished_at"`
	Results        []RelayTestCall `gorm:"serializer:json;type:jsonb;not null" json:"results"`
}

func (RelayTestRun) TableName() string { return "relay_test_runs" }
func (r *RelayStations) SaveTestRun(run *RelayTestRun) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var station RelayStation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&station, run.RelayStationID).Error; err != nil {
			return err
		}
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		retained := tx.Model(&RelayTestRun{}).Select("id").Where("relay_station_id = ?", run.RelayStationID).Order("finished_at DESC, id DESC").Limit(RelayTestHistoryLimit)
		return tx.Where("relay_station_id = ? AND id NOT IN (?)", run.RelayStationID, retained).Delete(&RelayTestRun{}).Error
	})
}
func (r *RelayStations) ListTestRuns(stationID uint) ([]RelayTestRun, error) {
	runs := make([]RelayTestRun, 0)
	err := r.db.Where("relay_station_id = ?", stationID).Order("finished_at DESC, id DESC").Limit(RelayTestHistoryLimit).Find(&runs).Error
	return runs, err
}
func (r *RelayStations) LatestTestRuns() ([]RelayTestRun, error) {
	runs := make([]RelayTestRun, 0)
	err := r.db.Raw(`SELECT DISTINCT ON (runs.relay_station_id) runs.* FROM relay_test_runs runs JOIN relay_stations stations ON stations.id = runs.relay_station_id WHERE stations.deleted_at IS NULL ORDER BY runs.relay_station_id, runs.finished_at DESC, runs.id DESC`).Scan(&runs).Error
	return runs, err
}
