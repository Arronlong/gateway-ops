package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDeletePelicanResultRouteScopesDeletionAndReturnsCorrectStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tt := range []struct {
		name   string
		path   string
		rows   int64
		dbErr  error
		status int
	}{
		{name: "delete selected artwork", path: "/relay-stations/7/accounts/20/pelican/results/31", rows: 1, status: http.StatusNoContent},
		{name: "missing artwork", path: "/relay-stations/7/accounts/20/pelican/results/31", status: http.StatusNotFound},
		{name: "database error", path: "/relay-stations/7/accounts/20/pelican/results/31", dbErr: errors.New("database unavailable"), status: http.StatusInternalServerError},
		{name: "invalid account", path: "/relay-stations/7/accounts/invalid/pelican/results/31", status: http.StatusBadRequest},
		{name: "zero account", path: "/relay-stations/7/accounts/0/pelican/results/31", status: http.StatusBadRequest},
		{name: "invalid station", path: "/relay-stations/invalid/accounts/20/pelican/results/31", status: http.StatusBadRequest},
		{name: "zero station", path: "/relay-stations/0/accounts/20/pelican/results/31", status: http.StatusBadRequest},
		{name: "invalid result", path: "/relay-stations/7/accounts/20/pelican/results/invalid", status: http.StatusBadRequest},
		{name: "zero result", path: "/relay-stations/7/accounts/20/pelican/results/0", status: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			if tt.status != http.StatusBadRequest {
				mock.ExpectBegin()
				expect := mock.ExpectExec(`DELETE FROM "relay_pelican_results" WHERE relay_station_id = \$1 AND account_external_id = \$2 AND id = \$3`).WithArgs(uint(7), int64(20), uint(31))
				if tt.dbErr != nil {
					expect.WillReturnError(tt.dbErr)
					mock.ExpectRollback()
				} else {
					expect.WillReturnResult(sqlmock.NewResult(0, tt.rows))
					mock.ExpectCommit()
				}
			}
			router := gin.New()
			registerRelayStations(router.Group(""), &Deps{RelayStations: storage.NewRelayStations(db)})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, tt.path, nil))
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.status, response.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
