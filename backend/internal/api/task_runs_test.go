package api

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTaskRunRoutesRejectInvalidInputs(t *testing.T) {
	router := gin.New()
	registerTaskRuns(router.Group(""), &Deps{})
	for _, tt := range []struct {
		path   string
		status int
	}{
		{"/tasks/unknown/runs", 404}, {"/tasks/channel_sync/runs?page=0", 400}, {"/tasks/channel_sync/runs?page_size=101", 400}, {"/tasks/channel_sync/runs?page=x", 400}, {"/tasks/channel_sync/runs/-1", 400}, {"/tasks/channel_sync/runs/0", 400},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if response.Code != tt.status {
			t.Fatalf("%s: %d %s", tt.path, response.Code, response.Body.String())
		}
	}
}
func TestTaskRunDetailIsScopedAndIncludesLogs(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT .* FROM "task_runs" WHERE task = .*AND id =`).WithArgs("channel_sync", uint(7), 1).WillReturnRows(sqlmock.NewRows([]string{"id", "task", "status", "logs", "started_at"}).AddRow(7, "channel_sync", "success", "scoped-log", time.Now()))
	router := gin.New()
	registerTaskRuns(router.Group(""), &Deps{DB: db})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasks/channel_sync/runs/7", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "scoped-log") {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
