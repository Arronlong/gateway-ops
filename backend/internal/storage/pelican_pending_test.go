package storage

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPelicanRunningResultDoesNotPruneHistory(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(31))
	mock.ExpectCommit()
	result := PelicanResult{RelayStationID: 7, AccountExternalID: 20, Status: "running"}
	if err := repo.SavePelicanResult(&result); err != nil {
		t.Fatal(err)
	}
	if result.ID != 31 {
		t.Fatalf("placeholder ID = %d", result.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanCompletionUpdatesExistingPlaceholder(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec(`UPDATE "relay_pelican_results" SET .* WHERE .*id`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .*app_settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.SavePelicanResult(&PelicanResult{ID: 31, RelayStationID: 7, AccountExternalID: 20, Status: "success", HTML: "<svg></svg>"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
