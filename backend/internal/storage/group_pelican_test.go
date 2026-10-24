package storage

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGroupPelicanArtifactKeepsGroupScope(t *testing.T) {
	value := GroupPelicanResult{ID: 12, RelayStationID: 7, GroupExternalID: 20, GroupName: "销售分组", Model: "model-a", Status: "success", HTML: "<svg/>", FinishedAt: time.Now()}
	result := value.Artifact()
	if result.GroupExternalID != 20 || result.AccountExternalID != 0 || result.AccountName != "销售分组" || result.HTML != "<svg/>" {
		t.Fatalf("wrong group scope: %#v", result)
	}
}

func TestGroupPelicanClaimHonorsLeaseAndAutomaticSwitch(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_group_pelican_configs".*lease_until.*enabled.*next_run_at`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	claimed, err := repo.ClaimGroupPelican(1, time.Now(), time.Now().Add(time.Minute), time.Now(), true)
	if err != nil || claimed {
		t.Fatalf("claimed=%v err=%v", claimed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGroupPelicanResultReadIsBoundToStationAndGroup(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectQuery(`SELECT .*relay_group_pelican_results.*relay_station_id = .*group_external_id = .*id =`).WithArgs(uint(7), int64(20), uint(12), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "group_external_id", "group_name", "html"}).AddRow(12, 7, 20, "销售分组", "<svg/>"))
	result, err := repo.GroupPelicanResult(7, 20, 12)
	if err != nil || result.GroupExternalID != 20 || result.HTML != "<svg/>" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
