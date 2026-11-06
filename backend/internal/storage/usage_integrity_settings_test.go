package storage

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelUsageIntegritySettingsDefaultsAndPersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "settings.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&AppSetting{}); err != nil {
		t.Fatal(err)
	}
	settings := NewSettings(db)
	current, err := settings.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !current.ChannelUsageIntegrityEnabled || current.ChannelUsageIntegrityRange != "24h" || current.ChannelUsageIntegrityTime != "07:00" {
		t.Fatalf("unexpected usage task defaults: %+v", current)
	}
	current.ChannelUsageIntegrityRange = "7d"
	current.ChannelUsageIntegrityTime = "08:45"
	if err := settings.SaveSyncSettings(current); err != nil {
		t.Fatal(err)
	}
	saved, err := settings.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if saved.ChannelUsageIntegrityRange != "7d" || saved.ChannelUsageIntegrityTime != "08:45" {
		t.Fatalf("settings did not persist: %+v", saved)
	}
	saved.ChannelUsageIntegrityRange = "30d"
	if err := settings.SaveSyncSettings(saved); err == nil {
		t.Fatal("unsupported range was accepted")
	}
	saved.ChannelUsageIntegrityRange = "7d"
	saved.ChannelUsageIntegrityTime = "25:99"
	if err := settings.SaveSyncSettings(saved); err == nil {
		t.Fatal("invalid time was accepted")
	}
}
