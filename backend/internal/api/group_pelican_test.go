package api

import (
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

func TestPublicGroupPelicanHasNoMutationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerPublicRelayStations(router.Group(""), &Deps{})
	for _, request := range [][2]string{{"POST", "/relay-stations/7/groups/20/pelican/run"}, {"PUT", "/relay-stations/7/groups/20/pelican"}, {"DELETE", "/relay-stations/7/groups/20/pelican/results/12"}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(request[0], request[1], nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("public mutation exposed: %v status=%d", request, response.Code)
		}
	}
}

func TestPublicGroupPelicanFollowsMonitorVisibilityForExclusiveGroups(t *testing.T) {
	for _, testCase := range []struct{ exclusive, monitored, allowed bool }{{true, true, true}, {true, false, false}, {false, false, false}} {
		sqlDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery(`SELECT .*relay_groups`).WillReturnRows(sqlmock.NewRows([]string{"relay_station_id", "external_id", "is_exclusive", "monitor_enabled"}).AddRow(7, 20, testCase.exclusive, testCase.monitored))
		if testCase.allowed {
			mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			mock.ExpectQuery(`SELECT .*relay_group_pelican_results`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "group_external_id", "status"}).AddRow(12, 7, 20, "success"))
		}
		router := gin.New()
		registerPublicRelayStations(router.Group(""), &Deps{RelayStations: storage.NewRelayStations(db)})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/relay-stations/7/groups/20/pelican/results/12", nil))
		want := http.StatusNotFound
		if testCase.allowed {
			want = http.StatusOK
		}
		if response.Code != want {
			t.Fatalf("monitor visibility mismatch: status=%d want=%d body=%s", response.Code, want, response.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		sqlDB.Close()
	}
}
