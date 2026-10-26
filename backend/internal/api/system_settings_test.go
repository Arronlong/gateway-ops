package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSystemSettingsRecentMetricsHTTPCompatibility(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "settings.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.AppSetting{}, &storage.PelicanConfig{}, &storage.GroupPelicanConfig{}); err != nil {
		t.Fatal(err)
	}
	settings := storage.NewSettings(db)
	if err := settings.Set(storage.SettingUIRecentMetricsWindowHours, "3"); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	registerSystemSettings(router.Group(""), &Deps{Settings: settings})
	request := func(method, body string, code, limit, hours int) {
		t.Helper()
		req := httptest.NewRequest(method, "/system-settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != code {
			t.Fatalf("%s %s: status %d, want %d: %s", method, body, response.Code, code, response.Body.String())
		}
		if code != 200 {
			return
		}
		var result struct {
			Data storage.UISettings `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Data.RecentMetricsLimit != limit || result.Data.RecentMetricsWindowHours != hours {
			t.Fatalf("%s %s: limit=%d hours=%d, want %d/%d", method, body, result.Data.RecentMetricsLimit, result.Data.RecentMetricsWindowHours, limit, hours)
		}
	}
	request("GET", "", 200, 60, 3)
	request("PUT", `{"recent_metrics_window_hours":3}`, 200, 60, 3)
	request("GET", "", 200, 60, 3)
	request("PUT", `{"recent_metrics_window_hours":12,"recent_metrics_limit":30}`, 200, 30, 12)
	request("GET", "", 200, 30, 12)
	request("PUT", `{"recent_metrics_window_hours":3}`, 200, 30, 3)
	for _, limit := range []string{"0", "-1", "101", "1.5", `"30"`} {
		request("PUT", `{"recent_metrics_limit":`+limit+`}`, 400, 0, 0)
	}
	request("GET", "", 200, 30, 3)
}

func TestValidateSystemSettingsRecentMetricsLimit(t *testing.T) {
	for _, limit := range []int{-1, 0, 1, 60, 100, 101} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			settings := storage.UISettings{RecentMetricsWindowHours: 3, IframeEmbeddingMode: storage.IframeEmbeddingModeAll, PelicanPrompt: storage.DefaultPelicanPrompt, PelicanReasoningEffort: "medium", PelicanParallelCount: 1, PelicanCronExpression: "*/30 * * * *", PelicanActiveStart: "08:00", PelicanActiveEnd: "22:00", PelicanHistoryCount: storage.PelicanHistoryLimit}
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"recent_metrics_limit":%d}`, limit)), &settings); err != nil {
				t.Fatal(err)
			}
			err := validateSystemSettings(settings)
			wantErr := limit < 1 || limit > 100
			if (err != nil) != wantErr {
				t.Fatalf("limit %d: error = %v, wantErr %v", limit, err, wantErr)
			}
		})
	}
}

func TestValidateSystemSettingsIframeConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings storage.UISettings
		wantErr  bool
	}{
		{
			name: "allow all",
			settings: storage.UISettings{
				RecentMetricsLimit:       60,
				RecentMetricsWindowHours: 1,
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAll,
			},
		},
		{
			name:     "window hours out of range",
			settings: storage.UISettings{RecentMetricsLimit: 60, RecentMetricsWindowHours: 25, IframeEmbeddingMode: storage.IframeEmbeddingModeAll},
			wantErr:  true,
		},
		{
			name:     "window hours below range",
			settings: storage.UISettings{RecentMetricsLimit: 60, RecentMetricsWindowHours: 0, IframeEmbeddingMode: storage.IframeEmbeddingModeAll},
			wantErr:  true,
		},
		{
			name: "valid allowlist",
			settings: storage.UISettings{
				RecentMetricsLimit:       60,
				RecentMetricsWindowHours: 1,
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"example.com", "*.example.com", "**.devai.**", "a*abc.com", "devai_cn.xxx.cn"},
			},
		},
		{
			name: "invalid mode",
			settings: storage.UISettings{
				RecentMetricsLimit:       60,
				RecentMetricsWindowHours: 1,
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid allowlist pattern",
			settings: storage.UISettings{
				RecentMetricsLimit:       60,
				RecentMetricsWindowHours: 1,
				IframeEmbeddingEnabled:   true,
				IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
				IframeEmbeddingAllowlist: []string{"https://example.com"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.settings.PelicanPrompt = storage.DefaultPelicanPrompt
			tt.settings.PelicanReasoningEffort = "medium"
			tt.settings.PelicanParallelCount = 1
			tt.settings.PelicanCronExpression = "*/30 * * * *"
			tt.settings.PelicanActiveStart = "08:00"
			tt.settings.PelicanActiveEnd = "22:00"
			tt.settings.PelicanHistoryCount = storage.PelicanHistoryLimit
			err := validateSystemSettings(tt.settings)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSystemSettings() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSystemSettingsPelicanConfiguration(t *testing.T) {
	t.Parallel()

	base := storage.UISettings{
		RecentMetricsLimit:       60,
		RecentMetricsWindowHours: 1,
		IframeEmbeddingMode:      storage.IframeEmbeddingModeAllowlist,
		PelicanReasoningEffort:   "medium",
		PelicanPrompt:            storage.DefaultPelicanPrompt,
		PelicanParallelCount:     1,
		PelicanCronExpression:    "*/30 * * * *",
		PelicanActiveStart:       "08:00",
		PelicanActiveEnd:         "22:00",
		PelicanHistoryCount:      storage.PelicanHistoryLimit,
	}
	tests := []struct {
		name    string
		update  func(*storage.UISettings)
		wantErr bool
	}{
		{name: "empty model is allowed before initial setup"},
		{name: "overnight window", update: func(s *storage.UISettings) { s.PelicanActiveStart = "22:00"; s.PelicanActiveEnd = "06:00" }},
		{name: "all day window", update: func(s *storage.UISettings) { s.PelicanActiveStart = "00:00"; s.PelicanActiveEnd = "00:00" }},
		{name: "invalid start", update: func(s *storage.UISettings) { s.PelicanActiveStart = "25:00" }, wantErr: true},
		{name: "empty end", update: func(s *storage.UISettings) { s.PelicanActiveEnd = "" }, wantErr: true},
		{name: "frequency outside window", update: func(s *storage.UISettings) { s.PelicanCronExpression = "0 23 * * *" }, wantErr: true},
		{name: "parallel count below range", update: func(s *storage.UISettings) { s.PelicanParallelCount = 0 }, wantErr: true},
		{name: "parallel count above range", update: func(s *storage.UISettings) { s.PelicanParallelCount = 9 }, wantErr: true},
		{name: "invalid reasoning effort", update: func(s *storage.UISettings) { s.PelicanReasoningEffort = "bogus" }, wantErr: true},
		{name: "invalid cron expression", update: func(s *storage.UISettings) { s.PelicanCronExpression = "invalid" }, wantErr: true},
		{name: "empty prompt", update: func(s *storage.UISettings) { s.PelicanPrompt = " " }, wantErr: true},
		{name: "history count below range", update: func(s *storage.UISettings) { s.PelicanHistoryCount = 0 }, wantErr: true},
		{name: "history count above range", update: func(s *storage.UISettings) { s.PelicanHistoryCount = storage.PelicanHistoryLimitMax + 1 }, wantErr: true},
		{name: "impossible schedule", update: func(s *storage.UISettings) { s.PelicanCronExpression = "0 0 31 2 *" }, wantErr: true},
		{name: "independent plans are valid", update: func(s *storage.UISettings) {
			s.PelicanPlans = []storage.PelicanTestPlan{{Model: "model-a,model-a-fallback", Prompt: "draw A", ReasoningEffort: "low"}, {Model: "model-b", Prompt: "draw B", ReasoningEffort: "high"}}
		}},
		{name: "each plan requires a model", update: func(s *storage.UISettings) {
			s.PelicanPlans = []storage.PelicanTestPlan{{Model: "model-a", Prompt: "draw A", ReasoningEffort: "low"}, {Prompt: "draw B", ReasoningEffort: "high"}}
		}, wantErr: true},
		{name: "each plan requires a prompt", update: func(s *storage.UISettings) {
			s.PelicanPlans = []storage.PelicanTestPlan{{Model: "model-a", Prompt: " ", ReasoningEffort: "low"}}
		}, wantErr: true},
		{name: "plan count is bounded", update: func(s *storage.UISettings) {
			s.PelicanPlans = make([]storage.PelicanTestPlan, 9)
			for i := range s.PelicanPlans {
				s.PelicanPlans[i] = storage.PelicanTestPlan{Model: "model", Prompt: "draw", ReasoningEffort: "medium"}
			}
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := base
			if tt.update != nil {
				tt.update(&settings)
			}
			err := validateSystemSettings(settings)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSystemSettings() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
