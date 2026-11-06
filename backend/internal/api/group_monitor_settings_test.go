package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/config"
	"github.com/guliansheng/gateway-ops/internal/scheduler"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupMonitorFrequencyAPIValidatesAndPreservesLegacyPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sync.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	repo := storage.NewSettings(db)
	sch := scheduler.New(config.SchedulerConfig{}, nil, nil, nil, nil, repo, nil, nil, nil)
	router := gin.New()
	registerSyncSettings(router.Group(""), &Deps{Scheduler: sch})
	for _, test := range []struct {
		body string
		want int
	}{{`{"group_monitor_interval_seconds":120}`, 200}, {`{}`, 200}, {`{"group_monitor_interval_seconds":9}`, 400}, {`{"group_monitor_interval_seconds":3601}`, 400}, {`{"group_monitor_interval_seconds":0}`, 400}} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/sync-settings", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s status=%d body=%s", test.body, response.Code, response.Body.String())
		}
		value, err := repo.GroupMonitorIntervalSeconds()
		if err != nil || value != 120 {
			t.Fatal(fmt.Sprintf("interval not preserved: %d %v", value, err))
		}
	}
}
