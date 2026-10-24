package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPelicanLibraryRouteFiltersAndReturnsPagedMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT count\(\*\) FROM \(SELECT results\.id`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	finished := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .*station_name.*FROM \(SELECT results\.id`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "account_external_id", "account_name", "model", "reasoning_effort", "trigger", "status", "error", "duration_ms", "started_at", "finished_at", "station_name"}).AddRow(31, 7, 20, "Account A", "model-a", "medium", "scheduled", "failed", "generation failed", 1234, finished.Add(-time.Second), finished, "Station A"))
	router := gin.New()
	registerRelayStations(router.Group(""), &Deps{RelayStations: storage.NewRelayStations(db)})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/relay-stations/pelican-results?station_id=7&status=failed&search=pelican&page=2&page_size=20", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data struct {
			Results []struct {
				StationName string `json:"station_name"`
				Status      string `json:"status"`
				HTML        string `json:"html"`
			} `json:"results"`
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Total != 21 || body.Data.Page != 2 || body.Data.PageSize != 20 || len(body.Data.Results) != 1 || body.Data.Results[0].StationName != "Station A" || body.Data.Results[0].Status != "failed" || body.Data.Results[0].HTML != "" {
		t.Fatalf("unexpected response: %#v", body.Data)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanLibraryRouteRejectsInvalidFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerRelayStations(router.Group(""), &Deps{})
	for _, query := range []string{"status=running", "station_id=0", "station_id=abc", "page=0", "page_size=101"} {
		t.Run(query, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/relay-stations/pelican-results?"+query, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("query=%s status=%d body=%s", query, response.Code, response.Body.String())
			}
		})
	}
}
