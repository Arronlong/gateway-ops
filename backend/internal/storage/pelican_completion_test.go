package storage

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"time"
)

func TestPelicanClaimPreservesDueTimeUntilCompletion(t *testing.T) {
	repo, mock := pelicanDB(t)
	now := time.Now().UTC()
	until := now.Add(time.Minute)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET "lease_until"=\$1,"updated_at"=\$2 WHERE .*next_run_at`).WithArgs(until, sqlmock.AnyArg(), uint(1), now, true, now, now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	claimed, err := repo.ClaimPelican(1, now, until, now, true)
	if err != nil || !claimed {
		t.Fatalf("claim=%v,%v", claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanClaimRejectsStaleDueSnapshot(t *testing.T) {
	repo, mock := pelicanDB(t)
	now := time.Now().UTC()
	expected := now.Add(-time.Minute)
	until := now.Add(time.Minute)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*next_run_at <= \$6 AND next_run_at = \$7`).WithArgs(until, sqlmock.AnyArg(), uint(1), now, true, now, expected).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	claimed, err := repo.ClaimPelican(1, now, until, expected, true)
	if err != nil || claimed {
		t.Fatalf("claim=%v,%v", claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanCompletionAdvancesScheduleWithLease(t *testing.T) {
	repo, mock := pelicanDB(t)
	next := time.Now().Add(time.Hour)
	until := time.Now().Add(time.Minute)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*next_run_at.*WHERE id = \$5 AND lease_until = \$6`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.CompletePelican(1, until, time.Now(), next); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
