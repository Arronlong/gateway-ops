package storage

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"testing"
)

func TestGroupMonitorIntervalDefaultsPersistenceAndBounds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "settings.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&AppSetting{}); err != nil {
		t.Fatal(err)
	}
	repo := NewSettings(db)
	settings, err := repo.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.GroupMonitorIntervalSeconds != 30 {
		t.Fatalf("default interval=%d", settings.GroupMonitorIntervalSeconds)
	}
	settings.GroupMonitorIntervalSeconds = 120
	if err := repo.SaveSyncSettings(settings); err != nil {
		t.Fatal(err)
	}
	restored, err := repo.SyncSettings()
	if err != nil || restored.GroupMonitorIntervalSeconds != 120 {
		t.Fatalf("saved interval=%+v %v", restored, err)
	}
	for _, invalid := range []int{-1, 9, 3601} {
		restored.GroupMonitorIntervalSeconds = invalid
		if err := repo.SaveSyncSettings(restored); err == nil {
			t.Fatalf("accepted invalid interval %d", invalid)
		}
	}
	value, err := repo.GroupMonitorIntervalSeconds()
	if err != nil || value != 120 {
		t.Fatalf("invalid write changed saved interval: %d %v", value, err)
	}
}
