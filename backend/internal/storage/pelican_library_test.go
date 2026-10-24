package storage

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPelicanLibraryFiltersLatestThirtyAndPaginatesWithoutArtwork(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectQuery(`SELECT count\(\*\) FROM \(SELECT results\.id`).
		WithArgs("sub2api", 5, uint(7), "failed", "%pelican%", "%pelican%", "%pelican%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(41))
	finished := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .*station_name.*FROM \(SELECT results\.id`).
		WithArgs("sub2api", 5, uint(7), "failed", "%pelican%", "%pelican%", "%pelican%", 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "account_external_id", "account_name", "model", "reasoning_effort", "trigger", "status", "error", "duration_ms", "started_at", "finished_at", "station_name", "upstream_group_name", "upstream_group_multiplier"}).
			AddRow(31, 7, 20, "pelican-account", "model-a", "medium", "scheduled", "failed", "upstream error", 1234, finished.Add(-time.Second), finished, "Station A", "历史 Key 分组", 0.25))
	page, err := repo.PelicanLibrary(PelicanLibraryFilter{StationID: 7, Status: "failed", Search: "pelican", Page: 2, PageSize: 20, HistoryCount: 5})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 41 || page.Page != 2 || page.PageSize != 20 || len(page.Results) != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	result := page.Results[0]
	if result.StationName != "Station A" || result.AccountName != "pelican-account" || result.Status != "failed" || result.HTML != "" {
		t.Fatalf("unexpected library record: %#v", result)
	}
	if result.UpstreamGroupName != "历史 Key 分组" || result.UpstreamGroupMultiplier == nil || *result.UpstreamGroupMultiplier != 0.25 {
		t.Fatalf("key group snapshot missing: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanLibraryBoundsPaginationAndRejectsInvalidStatus(t *testing.T) {
	repo, _ := pelicanDB(t)
	page, err := repo.PelicanLibrary(PelicanLibraryFilter{Page: -4, PageSize: 500})
	if err == nil || !regexp.MustCompile(`status`).MatchString(err.Error()) {
		t.Fatalf("invalid status accepted: page=%#v err=%v", page, err)
	}
	page, err = repo.PelicanLibrary(PelicanLibraryFilter{Page: -4, PageSize: 500, Status: "all"})
	if err == nil || page.Page != 1 || page.PageSize != 100 {
		t.Fatalf("pagination not normalized: page=%#v err=%v", page, err)
	}
}
