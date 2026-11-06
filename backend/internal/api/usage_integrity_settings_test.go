package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/config"
	"github.com/guliansheng/gateway-ops/internal/monitor"
	"github.com/guliansheng/gateway-ops/internal/scheduler"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSyncSettingsChannelUsageIntegrityRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sync-settings.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.AppSetting{}, &storage.TaskRun{}, &storage.Channel{}); err != nil {
		t.Fatal(err)
	}
	settings := storage.NewSettings(db)
	sch := scheduler.New(config.SchedulerConfig{}, nil, nil, nil, nil, settings, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := gin.New()
	registerSyncSettings(router.Group(""), &Deps{Scheduler: sch})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sync-settings", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"channel_usage_integrity_enabled":true`) || !strings.Contains(response.Body.String(), `"channel_usage_integrity_range":"24h"`) || !strings.Contains(response.Body.String(), `"channel_usage_integrity_time":"07:00"`) {
		t.Fatalf("default settings response: status=%d body=%s", response.Code, response.Body.String())
	}
	body := `{"channel_usage_integrity_enabled":false,"channel_usage_integrity_range":"7d","channel_usage_integrity_time":"08:45"}`
	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/sync-settings", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("update response: status=%d body=%s", response.Code, response.Body.String())
	}
	saved, err := settings.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if saved.ChannelUsageIntegrityEnabled || saved.ChannelUsageIntegrityRange != "7d" || saved.ChannelUsageIntegrityTime != "08:45" {
		t.Fatalf("settings not updated: %+v", saved)
	}
	for _, body := range []string{
		`{"channel_usage_integrity_enabled":true,"channel_usage_integrity_range":"30d","channel_usage_integrity_time":"07:00"}`,
		`{"channel_usage_integrity_enabled":true,"channel_usage_integrity_range":"24h","channel_usage_integrity_time":"25:99"}`,
	} {
		request = httptest.NewRequest(http.MethodPut, "/sync-settings", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid settings accepted: body=%s status=%d", body, response.Code)
		}
	}
}

func TestManualChannelUsageIntegrityRunCreatesTaskHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "manual-run.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.AppSetting{}, &storage.TaskRun{}, &storage.Channel{}); err != nil {
		t.Fatal(err)
	}
	settings := storage.NewSettings(db)
	monitorService := monitor.NewService(storage.NewChannels(db), nil, nil, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sch := scheduler.New(config.SchedulerConfig{}, monitorService, nil, nil, nil, settings, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := gin.New()
	registerTaskRuns(router.Group(""), &Deps{DB: db, Scheduler: sch})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/tasks/channel_usage_integrity/run", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("manual trigger status=%d body=%s", response.Code, response.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var run storage.TaskRun
		if err := db.Where("task = ?", "channel_usage_integrity").First(&run).Error; err == nil && run.Status != "running" {
			if run.Status != "success" {
				t.Fatalf("unexpected run: %+v", run)
			}
			if !strings.Contains(run.Logs, "0") {
				t.Fatalf("summary logs missing: %s", run.Logs)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("manual task run was not recorded as complete")
}
