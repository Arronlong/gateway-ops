package storage

import (
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBalanceTrendCustomWindowUsesBrowserTimeZonePostgres(t *testing.T) {
	dsn := os.Getenv("GATEWAYOPS_TEST_DSN")
	if dsn == "" {
		t.Skip("GATEWAYOPS_TEST_DSN is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	tx := db.Begin()
	defer tx.Rollback()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := tx.Exec("CREATE TEMP TABLE balance_snapshots (channel_id BIGINT NOT NULL, sampled_at TIMESTAMPTZ NOT NULL, balance DOUBLE PRECISION NOT NULL) ON COMMIT DROP").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("CREATE TEMP TABLE channels (id BIGINT PRIMARY KEY, deleted_at TIMESTAMPTZ, exclude_from_statistics BOOLEAN NOT NULL DEFAULT false) ON COMMIT DROP").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO channels (id, exclude_from_statistics) VALUES (1, false), (2, true)").Error; err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 2)
	for _, sample := range []struct {
		at    time.Time
		value float64
	}{{start.Add(-time.Minute), 999}, {start.Add(time.Hour), 100}, {start.Add(23 * time.Hour), 90}, {start.Add(25 * time.Hour), 80}, {end, 777}} {
		if err := tx.Exec("INSERT INTO balance_snapshots (channel_id, sampled_at, balance) VALUES (1,?,?)", sample.at, sample.value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec("INSERT INTO balance_snapshots (channel_id, sampled_at, balance) VALUES (2,?,999)", start.Add(23*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	rows, err := NewRates(tx).AggregateBalanceTrendWindow(start, end, "day", location)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].Day.Equal(start) || rows[0].Balance != 90 || !rows[1].Day.Equal(start.AddDate(0, 0, 1)) || rows[1].Balance != 80 {
		t.Fatalf("custom trend = %#v", rows)
	}
}
