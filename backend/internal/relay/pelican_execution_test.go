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
	return NewService(storage.NewRelayStations(db), nil), mock
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
