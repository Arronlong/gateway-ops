package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestChannelStatisticsMigrationDefaultsAndPreservesOptOut(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "upgrade.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.Exec("CREATE TABLE channels (id integer PRIMARY KEY, name text NOT NULL, type text NOT NULL, site_url text NOT NULL, username text NOT NULL, password_cipher text NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO channels VALUES (1, 'legacy', 'newapi', 'https://example.invalid', 'user', 'cipher')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatal(err)
	}
	var legacy Channel
	if err := db.First(&legacy, 1).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.ExcludeFromStatistics {
		t.Fatal("existing channels must participate by default")
	}
	legacy.ExcludeFromStatistics = true
	if err := db.Save(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Channel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&legacy, 1).Error; err != nil {
		t.Fatal(err)
	}
	if !legacy.ExcludeFromStatistics {
		t.Fatal("migration reset saved opt-out")
	}
}

func TestAggregateBalanceTrendsFilterExcludedChannels(t *testing.T) {
	for _, kind := range []string{"custom", "daily", "hourly"} {
		t.Run(kind, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			start := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
			mock.ExpectQuery("(?s)SELECT id FROM channels WHERE deleted_at IS NULL AND exclude_from_statistics = false").WillReturnRows(sqlmock.NewRows([]string{"day", "balance"}).AddRow(start, 100.0))
			rates := NewRates(db)
			var rows []DailyAggregate
			switch kind {
			case "custom":
				rows, err = rates.AggregateBalanceTrendWindow(start, start.Add(24*time.Hour), "day", time.UTC)
			case "daily":
				rows, err = rates.AggregateBalanceTrendSince(start)
			case "hourly":
				rows, err = rates.AggregateBalanceTrendHourlySince(start)
			}
			if err != nil || len(rows) != 1 || rows[0].Balance != 100 {
				t.Fatalf("rows=%+v err=%v", rows, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
