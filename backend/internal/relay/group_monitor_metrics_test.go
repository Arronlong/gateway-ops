package relay

import (
	"context"
	"encoding/json"
	"github.com/glebarez/sqlite"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestGroupRecentMetricsMatchChannelWeighting(t *testing.T) {
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	samples := groupMetricUsageSamples([]remoteUsage{
		{ID: 1, CreatedAt: now.Add(-time.Minute).Format(time.RFC3339), Model: "text", InputTokens: 100, CacheReadTokens: 900, OutputTokens: 100, DurationMS: 1000},
		{ID: 2, CreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339), Model: "text", InputTokens: 100, OutputTokens: 100, DurationMS: 9000},
	})
	got := buildGroupRecentMetrics(samples, now, 6, 60, true)
	if got.CacheHitRate == nil || math.Abs(*got.CacheHitRate-900.0/1100) > 1e-9 {
		t.Fatalf("cache rate = %+v", got)
	}
	if got.SuccessRate == nil || *got.SuccessRate != 1 {
		t.Fatalf("success rate = %+v", got)
	}
	if got.TPS == nil || *got.TPS != 20 || got.TPSScore == nil || *got.TPSScore != 40 {
		t.Fatalf("TPS = %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"samples", "calls", "request_id", "account_id", "user_id"} {
		if _, exists := data[key]; exists {
			t.Fatalf("private request detail exposed: %s", key)
		}
	}
}

func TestRemoteRequestErrorAcceptsNumericAndStringRequestTypes(t *testing.T) {
	var page remoteRequestErrorPage
	raw := `{"items":[{"request_type":2,"id":1},{"request_type":"probe","id":2},{"request_type":null,"id":3},{"id":4}]}`
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatalf("decode mixed request types: %v", err)
	}
	if len(page.Items) != 4 || string(page.Items[0].RequestType) != "2" || string(page.Items[1].RequestType) != "probe" || page.Items[2].RequestType != "" || page.Items[3].RequestType != "" {
		t.Fatalf("request types = %+v", page.Items)
	}
	if err := json.Unmarshal([]byte(`{"items":[{"request_type":true}]}`), &page); err == nil {
		t.Fatal("invalid request type must not be silently accepted")
	}
}

func TestPublicGroupRecentMetricsFollowSettingsAndKeepHistoryRange(t *testing.T) {
	now := time.Now().UTC()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("group_id") != "28" {
			t.Errorf("wrong group: %s", r.URL.String())
		}
		var data any
		if r.URL.Path == "/api/v1/admin/usage" {
			data = remoteUsagePage{Items: []remoteUsage{
				{ID: 1, CreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339), Model: "text", InputTokens: 100, CacheReadTokens: 100, OutputTokens: 100, DurationMS: 1000},
				{ID: 2, CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), Model: "text", InputTokens: 100, OutputTokens: 100, DurationMS: 9000},
				{ID: 3, CreatedAt: now.Add(-48 * time.Hour).Format(time.RFC3339), InputTokens: 99999},
			}}
		} else if r.URL.Path == "/api/v1/admin/ops/request-errors" {
			start, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("start_time"))
			if err != nil || start.After(now.Add(-24*time.Hour)) {
				t.Errorf("metric window coverage too narrow: %s", r.URL.String())
			}
			data = map[string]any{"items": []map[string]any{{"id": 4, "request_type": 2, "created_at": now.Add(-3 * time.Minute).Format(time.RFC3339), "error_owner": "provider", "status_code": 502, "message": "Recovered upstream error after retry"}}}
		} else {
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	defer server.Close()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "metrics.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.RelayStation{}, &storage.RelayGroup{}, &storage.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	cipher, _ := appcrypto.NewCipher("group-metric-test-secret")
	secret, _ := cipher.Encrypt("admin-key")
	station := storage.RelayStation{Name: "station", Type: "sub2api", BaseURL: server.URL, APIKeyCipher: secret}
	stations := storage.NewRelayStations(db)
	if err := stations.Create(&station); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.RelayGroup{RelayStationID: station.ID, ExternalID: 28, Name: "group", Status: "active", MonitorEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	settings := storage.NewSettings(db)
	if err := settings.Set(storage.SettingUIRecentMetricsWindowHours, "24"); err != nil {
		t.Fatal(err)
	}
	service := NewService(stations, cipher, nil, nil, settings)
	for _, limit := range []int{60, 1} {
		if err := settings.Set(storage.SettingUIRecentMetricsLimit, strconv.Itoa(limit)); err != nil {
			t.Fatal(err)
		}
		view, err := service.PublicGroupMonitorForRange(context.Background(), station.ID, "24h")
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Groups) != 1 {
			t.Fatalf("groups = %+v", view.Groups)
		}
		group := view.Groups[0]
		metrics := group.RecentMetrics
		if metrics.WindowHours != 24 || metrics.SampleLimit != limit || metrics.TPS == nil {
			t.Fatalf("configured metrics = %+v", metrics)
		}
		if group.SuccessCount != 2 || group.FailureCount != 0 || len(group.Calls) != 2 {
			t.Fatalf("metrics changed final-outcome history: %+v", group)
		}
		if limit == 60 && (metrics.SampleCount != 3 || *metrics.TPS != 20 || metrics.SuccessAttemptCount != 3) {
			t.Fatalf("aggregate = %+v", metrics)
		}
		if limit == 1 && (metrics.SampleCount != 1 || *metrics.TPS != 100) {
			t.Fatalf("settings reused stale cache: %+v", metrics)
		}
	}
	if requests.Load() != 4 {
		t.Fatalf("configuration did not invalidate aggregate cache: %d", requests.Load())
	}
}

func TestGroupRecentMetricsKeepRetryAttemptsAndExcludeAuxiliary(t *testing.T) {
	now := time.Now().UTC()
	usage := []remoteUsage{
		{ID: 1, CreatedAt: now.Add(-time.Minute).Format(time.RFC3339), RequestID: "completed", ClientRequestID: "same-client", OutputTokens: 100, DurationMS: 1000},
		{ID: 2, CreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339), RequestType: "probe", OutputTokens: 10000, DurationMS: 1000},
		{ID: 3, CreatedAt: now.Add(-3 * time.Minute).Format(time.RFC3339), RequestType: "image", Model: "gpt-image-1", OutputTokens: 10000, DurationMS: 1000},
	}
	failures := []remoteRequestError{
		{ID: 7, RequestID: "attempt", ClientRequestID: "same-client", CreatedAt: now.Add(-90 * time.Second).Format(time.RFC3339), GroupID: 28, ErrorOwner: "provider", StatusCode: 502, Message: "Recovered upstream error after retry"},
		{ID: 8, CreatedAt: now.Add(-time.Minute).Format(time.RFC3339), GroupID: 29, ErrorOwner: "provider", StatusCode: 500, Message: "wrong group"},
		{ID: 9, CreatedAt: now.Add(-time.Minute).Format(time.RFC3339), GroupID: 28, ErrorOwner: "client", StatusCode: 400, Message: "invalid request"},
	}
	samples := append(groupMetricUsageSamples(usage), groupMetricErrorSamples(failures, 28)...)
	got := buildGroupRecentMetrics(samples, now, 1, 60, true)
	if got.SuccessAttemptCount != 3 || got.SuccessCount != 2 || got.SuccessRate == nil || math.Abs(*got.SuccessRate-2.0/3) > 1e-9 {
		t.Fatalf("retry-aware success = %+v", got)
	}
	if got.TPS == nil || *got.TPS != 100 {
		t.Fatalf("non-text samples affected TPS: %+v", got)
	}
}

func TestGroupRecentMetricsWindowLimitAndUnknownData(t *testing.T) {
	now := time.Now().UTC()
	usage := make([]remoteUsage, 0, 75)
	for i := 0; i < 70; i++ {
		usage = append(usage, remoteUsage{ID: int64(i + 1), CreatedAt: now.Add(-time.Duration(i+1) * time.Minute).Format(time.RFC3339), InputTokens: 10})
	}
	usage = append(usage, remoteUsage{ID: 100, CreatedAt: now.Add(-10 * time.Second).Format(time.RFC3339), CacheReadTokens: 99999}, remoteUsage{ID: 101, CreatedAt: "invalid"})
	samples := groupMetricUsageSamples(usage)
	got := buildGroupRecentMetrics(samples, now, 1, 20, true)
	if got.SampleCount != 20 || got.CacheHitRate == nil || *got.CacheHitRate != 0 || got.TPS != nil {
		t.Fatalf("window selection = %+v", got)
	}
	if got := buildGroupRecentMetrics(samples, now, 24, 200, true); got.SampleCount != 70 {
		t.Fatalf("full window sample count = %d", got.SampleCount)
	}
	for _, complete := range []bool{false, true} {
		got := buildGroupRecentMetrics(nil, now, 1, 60, complete)
		if got.CacheHitRate != nil || got.SuccessRate != nil || got.TPS != nil {
			t.Fatalf("missing data should be unknown: %+v", got)
		}
	}
	if got := buildGroupRecentMetrics(samples, now, 1, 60, false); got.CacheHitRate != nil || got.SuccessRate != nil || got.TPS != nil {
		t.Fatalf("partial data must not claim valid rates: %+v", got)
	}
}
