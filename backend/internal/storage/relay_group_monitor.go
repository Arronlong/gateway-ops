package storage

import (
	"context"
	"gorm.io/gorm/clause"
	"time"
)

// RelayGroupMonitorDay 持久保存单个分组一天的压缩明细，仅供后台聚合。
type RelayGroupMonitorDay struct {
	RelayStationID  uint      `gorm:"primaryKey"`
	GroupExternalID int64     `gorm:"primaryKey"`
	Day             time.Time `gorm:"primaryKey"`
	SourceKey       string    `gorm:"size:64;not null"`
	Payload         []byte    `gorm:"not null"`
	CollectedUntil  time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (RelayGroupMonitorDay) TableName() string { return "relay_group_monitor_days" }

// RelayGroupMonitorSnapshot 是公开接口可直接返回的小型安全投影。
type RelayGroupMonitorSnapshot struct {
	RelayStationID uint      `gorm:"primaryKey"`
	RangeName      string    `gorm:"primaryKey;size:16"`
	SourceKey      string    `gorm:"size:64;not null"`
	Payload        []byte    `gorm:"not null"`
	LastError      string    `gorm:"type:text;not null;default:''"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (RelayGroupMonitorSnapshot) TableName() string { return "relay_group_monitor_snapshots" }

func (r *RelayStations) GetGroupMonitorDay(ctx context.Context, stationID uint, groupID int64, day time.Time) (*RelayGroupMonitorDay, error) {
	var row RelayGroupMonitorDay
	err := r.db.WithContext(ctx).Where("relay_station_id = ? AND group_external_id = ? AND day = ?", stationID, groupID, day.UTC()).Take(&row).Error
	return &row, err
}
func (r *RelayStations) SaveGroupMonitorDay(ctx context.Context, row *RelayGroupMonitorDay) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "relay_station_id"}, {Name: "group_external_id"}, {Name: "day"}}, DoUpdates: clause.AssignmentColumns([]string{"source_key", "payload", "collected_until", "updated_at"})}).Create(row).Error
}
func (r *RelayStations) GetGroupMonitorSnapshot(ctx context.Context, stationID uint, preset string) (*RelayGroupMonitorSnapshot, error) {
	var row RelayGroupMonitorSnapshot
	err := r.db.WithContext(ctx).Where("relay_station_id = ? AND range_name = ?", stationID, preset).Take(&row).Error
	return &row, err
}
func (r *RelayStations) SaveGroupMonitorSnapshot(ctx context.Context, row *RelayGroupMonitorSnapshot) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "relay_station_id"}, {Name: "range_name"}}, DoUpdates: clause.AssignmentColumns([]string{"source_key", "payload", "last_error", "updated_at"})}).Create(row).Error
}
func (r *RelayStations) SetGroupMonitorSnapshotError(ctx context.Context, stationID uint, message string, attempt time.Time) error {
	return r.db.WithContext(ctx).Model(&RelayGroupMonitorSnapshot{}).Where("relay_station_id = ? AND updated_at < ?", stationID, attempt.UTC().Truncate(time.Microsecond)).Update("last_error", message).Error
}
func (r *RelayStations) PruneGroupMonitorDays(ctx context.Context, stationID uint, before time.Time) error {
	return r.db.WithContext(ctx).Where("relay_station_id = ? AND day < ?", stationID, before).Delete(&RelayGroupMonitorDay{}).Error
}

func (r *RelayStations) ListGroupMonitorStations(ctx context.Context) ([]RelayStation, error) {
	var rows []RelayStation
	err := r.db.WithContext(ctx).Order("id ASC").Find(&rows).Error
	return rows, err
}
func (r *RelayStations) GroupMonitorStation(ctx context.Context, id uint) (*RelayStation, error) {
	var row RelayStation
	err := r.db.WithContext(ctx).First(&row, id).Error
	return &row, err
}
func (r *RelayStations) ListMonitorGroups(ctx context.Context, stationID uint) ([]RelayGroup, error) {
	var rows []RelayGroup
	err := r.db.WithContext(ctx).Where("relay_station_id = ?", stationID).Order("sort_order ASC, external_id ASC").Find(&rows).Error
	return rows, err
}
