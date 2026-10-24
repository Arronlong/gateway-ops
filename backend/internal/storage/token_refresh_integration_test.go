package storage

import (
	"errors"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTokenRefreshSettingsPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("GATEWAYOPS_TEST_DSN")
	if dsn == "" {
		t.Skip("GATEWAYOPS_TEST_DSN is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	rollback := errors.New("rollback isolated token refresh settings")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE SCHEMA token_refresh_settings_test").Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL search_path TO token_refresh_settings_test").Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&AppSetting{}); err != nil {
			return err
		}
		repo := NewSettings(tx)
		current, err := repo.SyncSettings()
		if err != nil {
			return err
		}
		if current.ChannelTokenRefreshEnabled || current.ChannelTokenRefreshIntervalMinutes != 30 {
			t.Fatal("incorrect defaults")
		}
		current.ChannelTokenRefreshEnabled = true
		current.ChannelTokenRefreshIntervalMinutes = 47
		if err := repo.SaveSyncSettings(current); err != nil {
			return err
		}
		saved, err := repo.SyncSettings()
		if err != nil {
			return err
		}
		if !saved.ChannelTokenRefreshEnabled || saved.ChannelTokenRefreshIntervalMinutes != 47 {
			t.Fatal("settings did not persist")
		}
		saved.ChannelTokenRefreshEnabled = false
		if err := repo.SaveSyncSettings(saved); err != nil {
			return err
		}
		saved, err = repo.SyncSettings()
		if err != nil {
			return err
		}
		if saved.ChannelTokenRefreshEnabled || saved.ChannelTokenRefreshIntervalMinutes != 47 {
			t.Fatal("disabled settings did not persist")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
