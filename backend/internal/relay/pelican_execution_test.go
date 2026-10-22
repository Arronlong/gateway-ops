package relay

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func pelicanExecutionDB(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(storage.NewRelayStations(db), nil)
	service.settings = storage.NewSettings(db)
	return service, mock
}

func expectGlobalPelicanSettings(mock sqlmock.Sqlmock, model string) {
	for _, field := range [][2]string{{storage.SettingPelicanModel, model}, {storage.SettingPelicanPrompt, "global drawing"}, {storage.SettingPelicanReasoningEffort, "high"}, {storage.SettingPelicanCronExpression, "0 * * * *"}, {storage.SettingPelicanParallelCount, "1"}} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(field[0], 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(field[0], field[1]))
	}
}

func TestSavePelicanAccountUsesGlobalSettingsAndOnlyAccountSwitch(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "type"}).AddRow(7, "sub2api"))
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_configs"`).WithArgs(uint(7), int64(20), true, "global-model", "global drawing", "high", 1, "0 * * * *", sqlmock.AnyArg(), nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id", "enabled"}).AddRow(1, true))
	config, err := service.SavePelicanConfig(7, 20, storage.PelicanConfig{Enabled: true, Model: "must-not-override-global", Prompt: "ignored", ParallelCount: 8})
	if err != nil || !config.Enabled {
		t.Fatalf("config=%#v err=%v", config, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManualPelicanUsesGlobalSettingsEvenWhenAccountScheduleDisabled(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "type"}).AddRow(7, "sub2api"))
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id", "enabled", "model", "prompt", "parallel_count"}).AddRow(1, false, "legacy-model", "legacy", 8))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnError(errors.New("station removed after claim"))
	config, err := service.StartPelican(context.Background(), 7, 20)
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.Model != "global-model" || config.Prompt != "global drawing" || config.ParallelCount != 1 || config.LeaseUntil == nil {
		t.Fatalf("manual execution must keep schedule disabled and use global settings: %#v", config)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err := mock.ExpectationsWereMet()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScheduledPelicanSkipsGenerationUntilGlobalModelConfigured(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	expectGlobalPelicanSettings(mock, "")
	if err := service.RunDuePelicans(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManualPelicanCreatesDisabledAccountConfigOnFirstRun(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	stationRows := func() *sqlmock.Rows { return sqlmock.NewRows([]string{"id", "type"}).AddRow(7, "sub2api") }
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(stationRows())
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(stationRows())
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(stationRows())
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_configs"`).WithArgs(uint(7), int64(20), false, "global-model", "global drawing", "high", 1, "0 * * * *", sqlmock.AnyArg(), nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id", "enabled"}).AddRow(1, false))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnError(errors.New("station removed after claim"))
	config, err := service.StartPelican(context.Background(), 7, 20)
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.Model != "global-model" || config.LeaseUntil == nil {
		t.Fatalf("first manual run must not enable the account schedule: %#v", config)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err := mock.ExpectationsWereMet()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScheduledPelicanLoadsGlobalParametersInsteadOfLegacyAccountParameters(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectQuery(`SELECT c\.\* FROM relay_pelican_configs c`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "account_external_id", "enabled", "model", "prompt", "parallel_count", "cron_expression", "next_run_at"}).AddRow(1, 7, 20, true, "legacy-model", "legacy drawing", 8, "invalid-legacy-cron", now.Add(-time.Minute)))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WithArgs(uint(7), int64(20), "\u8d26\u53f7 #20", "global-model", "global drawing", "high", "scheduled", "failed", "", "record not found", int64(0), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.RunDuePelicans(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanMissingAccountCreatesFailureHistory(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	config := storage.PelicanConfig{ID: 1, RelayStationID: 7, AccountExternalID: 20, Model: "test-model", Prompt: "draw", ReasoningEffort: "medium", ParallelCount: 1, NextRunAt: time.Now().Add(-time.Minute)}
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WithArgs(uint(7), int64(20), "账号 #20", "test-model", "draw", "medium", "scheduled", "failed", "", "record not found", int64(0), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at.*WHERE id = \$5 AND lease_until = \$6`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	service.runPelicanBatch(context.Background(), config, time.Now().Add(time.Minute), time.Now().Add(time.Hour), "scheduled")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanFailedPersistenceKeepsLeaseAndDueTimeForRecovery(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	config := storage.PelicanConfig{ID: 1, RelayStationID: 7, AccountExternalID: 20, ParallelCount: 1}
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WillReturnError(errors.New("database temporarily unavailable"))
	mock.ExpectRollback()
	service.runPelicanBatch(context.Background(), config, time.Now().Add(time.Minute), time.Now().Add(time.Hour), "scheduled")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
