package storage

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestDeletedRunningPelicanResultCannotBeRecreatedByCompletion(t *testing.T) {
	for _, scope := range []string{"channel", "account", "group"} {
		t.Run(scope, func(t *testing.T) {
			repo, mock := pelicanDB(t)
			table := "relay_pelican_results"
			if scope == "group" {
				table = "relay_group_pelican_results"
			}
			mock.ExpectBegin()
			mock.ExpectExec(`DELETE FROM "` + table + `" WHERE .*`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			var err error
			switch scope {
			case "channel":
				err = repo.DeleteChannelPelicanResult(2, 9)
			case "account":
				err = repo.DeletePelicanResult(4, 3, 9)
			case "group":
				err = repo.DeleteGroupPelicanResult(4, 6, 9)
			}
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectBegin()
			if scope != "channel" {
				mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
			}
			mock.ExpectExec(`UPDATE "` + table + `" SET .*`).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectRollback()
			if scope == "group" {
				err = repo.SaveGroupPelicanResult(&GroupPelicanResult{ID: 9, RelayStationID: 4, GroupExternalID: 6, Status: "success", HTML: "<svg/>"})
			} else {
				result := PelicanResult{ID: 9, RelayStationID: 4, AccountExternalID: 3, Status: "success", HTML: "<svg/>"}
				if scope == "channel" {
					result.ChannelID = 2
					result.RelayStationID = 0
				}
				err = repo.SavePelicanResult(&result)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("late completion must not recreate deleted result: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
