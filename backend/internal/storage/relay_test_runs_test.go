package storage

import (
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"regexp"
	"testing"
	"time"
)

func historyDB(t *testing.T) (*RelayStations, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return NewRelayStations(db), mock
}
func TestSaveTestRunLocksStationAndPrunesWithinSameTransaction(t *testing.T) {
	repo, mock := historyDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WithArgs(uint(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "station"))
	mock.ExpectQuery(`INSERT INTO "relay_test_runs"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(31))
	mock.ExpectExec(`DELETE FROM "relay_test_runs" WHERE relay_station_id = \$1 AND id NOT IN \(SELECT "id" FROM "relay_test_runs" WHERE relay_station_id = \$2 ORDER BY finished_at DESC, id DESC LIMIT \$3\)`).WithArgs(uint(7), uint(7), RelayTestHistoryLimit).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	run := RelayTestRun{RelayStationID: 7, StartedAt: time.Now(), FinishedAt: time.Now(), Results: []RelayTestCall{{Index: 1, Success: true}}}
	if err := repo.SaveTestRun(&run); err != nil {
		t.Fatal(err)
	}
	if run.ID != 31 {
		t.Fatalf("id = %d", run.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSaveTestRunRollsBackWhenRetentionFails(t *testing.T) {
	repo, mock := historyDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_test_runs"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(31))
	mock.ExpectExec(`DELETE FROM "relay_test_runs"`).WillReturnError(errors.New("retention failed"))
	mock.ExpectRollback()
	if err := repo.SaveTestRun(&RelayTestRun{RelayStationID: 7, Results: []RelayTestCall{}}); err == nil {
		t.Fatal("expected rollback")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestHistoryReadIsStationScopedAndDecodesResults(t *testing.T) {
	repo, mock := historyDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "relay_test_runs" WHERE relay_station_id = $1 ORDER BY finished_at DESC, id DESC LIMIT $2`)).WithArgs(uint(7), 30).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "results"}).AddRow(31, 7, `[{"index":1,"success":true,"status_code":200,"duration_ms":12,"output":"ok"}]`))
	runs, err := repo.ListTestRuns(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || len(runs[0].Results) != 1 || runs[0].Results[0].Output != "ok" {
		t.Fatalf("runs = %#v", runs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestDeleteStationLocksBeforeCleaningHistory(t *testing.T) {
	repo, mock := historyDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WithArgs(uint(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	expected := errors.New("cleanup failed")
	mock.ExpectExec(`UPDATE "operation_ledger_entries"`).WillReturnError(expected)
	mock.ExpectRollback()
	if err := repo.Delete(7); !errors.Is(err, expected) {
		t.Fatalf("delete error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLatestHistoryReadDecodesJSON(t *testing.T) {
	repo, mock := historyDB(t)
	mock.ExpectQuery(`SELECT DISTINCT ON.*JOIN relay_stations.*deleted_at IS NULL`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "results"}).AddRow(31, 7, `[{"index":1,"success":false,"status_code":503,"output":"error"}]`))
	runs, err := repo.LatestTestRuns()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || len(runs[0].Results) != 1 || runs[0].Results[0].Success {
		t.Fatalf("runs = %#v", runs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
