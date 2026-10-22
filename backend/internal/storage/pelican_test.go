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

func pelicanDB(t *testing.T) (*RelayStations, sqlmock.Sqlmock) {
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
func TestPelicanRetentionIsPerStationAndAccount(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WithArgs(uint(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(31))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results" WHERE relay_station_id = \$1 AND account_external_id = \$2 AND id NOT IN`).WithArgs(uint(7), int64(20), uint(7), int64(20), 30).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.SavePelicanResult(&PelicanResult{RelayStationID: 7, AccountExternalID: 20}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestPelicanRetentionFailureRollsBack(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnError(errors.New("retention failure"))
	mock.ExpectRollback()
	if repo.SavePelicanResult(&PelicanResult{RelayStationID: 7, AccountExternalID: 20}) == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestPelicanLeaseCannotBeClaimedTwiceOrAfterDisable(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*lease_until.*enabled.*next_run_at`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	claimed, err := repo.ClaimPelican(1, time.Now(), time.Now().Add(time.Minute), time.Now(), true)
	if err != nil || claimed {
		t.Fatalf("claim = %v,%v", claimed, err)
	}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*WHERE id = \$3 AND lease_until = \$4`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := repo.FinishPelican(1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestPelicanResultReadRequiresStationAccountAndResult(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "relay_pelican_results" WHERE relay_station_id = $1 AND account_external_id = $2 AND id = $3 ORDER BY "relay_pelican_results"."id" LIMIT $4`)).WithArgs(uint(7), int64(20), uint(31), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "html"}).AddRow(31, "<svg></svg>"))
	result, err := repo.PelicanResult(7, 20, 31)
	if err != nil || result.HTML != "<svg></svg>" {
		t.Fatalf("result=%#v,%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanResultDeleteRequiresStationAccountAndResult(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "relay_pelican_results" WHERE relay_station_id = $1 AND account_external_id = $2 AND id = $3`)).WithArgs(uint(7), int64(20), uint(31)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.DeletePelicanResult(7, 20, 31); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanResultDeleteReturnsNotFoundForWrongOwnerOrMissingResult(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "relay_pelican_results" WHERE relay_station_id = \$1 AND account_external_id = \$2 AND id = \$3`).WithArgs(uint(9), int64(99), uint(31)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := repo.DeletePelicanResult(9, 99, 31); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("delete error = %v, want record not found", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanResultDeletePropagatesDatabaseFailure(t *testing.T) {
	repo, mock := pelicanDB(t)
	wantErr := errors.New("database unavailable")
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnError(wantErr)
	mock.ExpectRollback()
	if err := repo.DeletePelicanResult(7, 20, 31); !errors.Is(err, wantErr) {
		t.Fatalf("delete error = %v, want %v", err, wantErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestPelicanDeleteLocksStationFirst(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WithArgs(uint(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec(`UPDATE "operation_ledger_entries"`).WillReturnError(errors.New("cleanup failed"))
	mock.ExpectRollback()
	if repo.Delete(7) == nil {
		t.Fatal("expected cleanup failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
