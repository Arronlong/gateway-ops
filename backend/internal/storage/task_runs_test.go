package storage

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTaskRunsListScopesTaskAndOmitsLogs(t *testing.T) {
	repo, mock := pelicanDB(t)
	runs := NewTaskRuns(repo.db)
	mock.ExpectQuery(`SELECT .*count.*FROM "task_runs" WHERE task =`).WithArgs("channel_sync").WillReturnRows(sqlmock.NewRows([]string{"total", "succeeded", "failed", "running"}).AddRow(22, 20, 1, 1))
	mock.ExpectQuery(`SELECT .* FROM "task_runs" WHERE task = .*ORDER BY started_at DESC,id DESC LIMIT .*OFFSET`).WithArgs("channel_sync", 10, 10).WillReturnRows(sqlmock.NewRows([]string{"id", "task", "status", "started_at"}).AddRow(1, "channel_sync", "success", time.Now()))
	page, err := runs.List("channel_sync", 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 22 || page.Succeeded != 20 || page.Running != 1 || len(page.Runs) != 1 || page.Page != 2 {
		t.Fatalf("unexpected page: %#v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTaskRunDetailRequiresMatchingTask(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectQuery(`SELECT .* FROM "task_runs" WHERE .*task = .*AND id =`).WithArgs("relay_rate_sync", uint(9), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := NewTaskRuns(repo.db).Find("relay_rate_sync", 9); err == nil {
		t.Fatal("expected missing run")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
