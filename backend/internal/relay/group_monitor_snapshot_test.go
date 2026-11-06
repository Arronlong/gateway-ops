package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func monitorSnapshotFixture(t *testing.T, handler http.HandlerFunc) (*Service, *storage.RelayStation, *gorm.DB) {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "monitor.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.RelayStation{}, &storage.RelayGroup{}, &storage.AppSetting{}, &storage.RelayGroupMonitorDay{}, &storage.RelayGroupMonitorSnapshot{}); err != nil {
		t.Fatal(err)
	}
	cipher, _ := appcrypto.NewCipher("monitor-snapshot-test-secret")
	secret, _ := cipher.Encrypt("admin-key")
	station := &storage.RelayStation{Name: "station", Type: "sub2api", BaseURL: upstream.URL, APIKeyCipher: secret}
	repo := storage.NewRelayStations(db)
	if err := repo.Create(station); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.RelayGroup{RelayStationID: station.ID, ExternalID: 28, Name: "group", Status: "active", MonitorEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	return NewService(repo, cipher, nil, nil, storage.NewSettings(db)), station, db
}

func TestMonitorSnapshotColdAndRestartReadsNeverCallUpstream(t *testing.T) {
	var requests atomic.Int32
	service, station, _ := monitorSnapshotFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	for _, preset := range []string{"today", "24h", "7d", "30d"} {
		view, err := service.CachedPublicGroupMonitorForRange(context.Background(), station.ID, preset)
		if err != nil {
			t.Fatal(err)
		}
		if view.Range != preset || view.SyncStatus != "preparing" || len(view.Groups) != 1 || view.Groups[0].DataComplete {
			t.Fatalf("cold view = %+v", view)
		}
		view.SyncStatus = "ready"
		view.Groups[0].DataComplete = true
		view.Groups[0].SuccessCount = 42
		payload, _ := json.Marshal(view)
		if err := service.stations.SaveGroupMonitorSnapshot(context.Background(), &storage.RelayGroupMonitorSnapshot{RelayStationID: station.ID, RangeName: preset, SourceKey: monitorSourceKey(station), Payload: payload, UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	restarted := NewService(service.stations, service.cipher, nil, nil, service.settings)
	for _, preset := range []string{"today", "24h", "7d", "30d"} {
		view, err := restarted.CachedPublicGroupMonitorForRange(context.Background(), station.ID, preset)
		if err != nil || view.SyncStatus != "ready" || view.Groups[0].SuccessCount != 42 {
			t.Fatalf("persisted view = %+v, %v", view, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("foreground made %d upstream requests", requests.Load())
	}
}

func TestMonitorSnapshotRefreshAllRangesReusesHistoryAndKeepsFailureData(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	var requests atomic.Int32
	var fail atomic.Bool
	records := []remoteUsage{
		{ID: 1, CreatedAt: now.Add(-time.Hour).UTC().Format(time.RFC3339), Model: "text", DurationMS: 1000, OutputTokens: 100},
		{ID: 2, CreatedAt: now.Add(-48 * time.Hour).UTC().Format(time.RFC3339), Model: "text", DurationMS: 2000, OutputTokens: 100},
		{ID: 3, CreatedAt: now.Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339), Model: "text", DurationMS: 3000, OutputTokens: 100},
	}
	service, station, db := monitorSnapshotFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if fail.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Query().Get("group_id") != "28" {
			t.Errorf("unexpected group query: %s", r.URL.RawQuery)
		}
		items := []remoteUsage{}
		if r.URL.Path == "/api/v1/admin/usage" {
			day := r.URL.Query().Get("start_date")
			for _, record := range records {
				if record.CreatedAt[:10] == day {
					items = append(items, record)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": items, "pages": 1, "total": len(items)}})
		} else {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[],"total":0}}`)
		}
	})
	if err := service.refreshGroupMonitorSnapshotsAt(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		preset string
		count  int
	}{{"today", 1}, {"24h", 1}, {"7d", 2}, {"30d", 3}} {
		row, err := service.stations.GetGroupMonitorSnapshot(context.Background(), station.ID, test.preset)
		if err != nil {
			t.Fatal(err)
		}
		var view PublicGroupMonitor
		if err := json.Unmarshal(row.Payload, &view); err != nil {
			t.Fatal(err)
		}
		if view.SyncStatus != "ready" || !view.Groups[0].DataComplete || view.Groups[0].SuccessCount != test.count {
			t.Fatalf("%s view = %+v", test.preset, view)
		}
	}
	first := requests.Load()
	if err := service.refreshGroupMonitorSnapshotsAt(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if delta := requests.Load() - first; delta != 2 {
		t.Fatalf("refresh reread historical days: requests=%d want 2", delta)
	}
	fail.Store(true)
	if err := service.refreshGroupMonitorSnapshotsAt(context.Background(), now.Add(2*time.Minute)); err == nil {
		t.Fatal("expected sync failure")
	}
	row, err := service.stations.GetGroupMonitorSnapshot(context.Background(), station.ID, "30d")
	if err != nil || row.LastError == "" {
		t.Fatalf("failure state = %+v, %v", row, err)
	}
	var saved PublicGroupMonitor
	_ = json.Unmarshal(row.Payload, &saved)
	if saved.Groups[0].SuccessCount != 3 {
		t.Fatalf("failure erased snapshot: %+v", saved)
	}
	var groups []storage.RelayGroup
	db.Where("relay_station_id = ?", station.ID).Find(&groups)
	if err := db.Model(&storage.RelayGroup{}).Where("id = ?", groups[0].ID).Update("monitor_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	view, err := service.CachedPublicGroupMonitorForRange(context.Background(), station.ID, "30d")
	if err != nil || len(view.Groups) != 0 {
		t.Fatalf("disabled group remained public: %+v %v", view, err)
	}
}

func TestMonitorDayDoesNotTrustIncompletePagination(t *testing.T) {
	now := time.Now().UTC()
	service, station, _ := monitorSnapshotFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/usage" {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[]}}`)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []remoteUsage{{ID: 3, CreatedAt: now.Add(-time.Minute).Format(time.RFC3339)}}, "total": 3, "pages": 2}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": []remoteUsage{{ID: 1, CreatedAt: now.Add(-time.Minute).Format(time.RFC3339)}, {ID: 2, CreatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339)}}, "total": 3, "pages": 2}})
	})
	payload, err := service.loadOrRefreshMonitorDay(context.Background(), station, "admin-key", 28, now.Truncate(24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Usage) != 3 {
		t.Fatalf("silently truncated upstream rows: %d want 3", len(payload.Usage))
	}
}

func TestMonitorSnapshotStatusChangesAndCancellation(t *testing.T) {
	service, station, db := monitorSnapshotFixture(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not used", 500) })
	group := PublicGroupMonitorGroup{ExternalID: 28, GroupStatus: "disabled", Status: "disabled", DataComplete: true, SuccessCount: 42}
	payload, _ := json.Marshal(PublicGroupMonitor{StationID: station.ID, Range: "24h", UpdatedAt: time.Now(), SyncStatus: "ready", Groups: []PublicGroupMonitorGroup{group}})
	if err := service.stations.SaveGroupMonitorSnapshot(context.Background(), &storage.RelayGroupMonitorSnapshot{RelayStationID: station.ID, RangeName: "24h", SourceKey: monitorSourceKey(station), Payload: payload, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	view, err := service.CachedPublicGroupMonitorForRange(context.Background(), station.ID, "24h")
	if err != nil || view.Groups[0].Status == "disabled" || view.Groups[0].SuccessCount != 42 {
		t.Fatalf("reactivated group = %+v %v", view, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.refreshGroupMonitorSnapshotsAt(ctx, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled database read = %v", err)
	}
	var row storage.RelayStation
	db.First(&row, station.ID)
	row.BaseURL = "https://changed.invalid"
	db.Save(&row)
	view, err = service.CachedPublicGroupMonitorForRange(context.Background(), station.ID, "24h")
	if err != nil || view.SyncStatus != "preparing" || view.Groups[0].SuccessCount != 0 {
		t.Fatalf("changed source reused old counts = %+v %v", view, err)
	}
}

func TestMonitorHistoryFailureDoesNotMarkFreshShortRangesStale(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	var fail atomic.Bool
	service, station, db := monitorSnapshotFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() && r.URL.Query().Get("start_date") == "2026-10-20" {
			http.Error(w, "old history unavailable", 502)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"items":[]}}`)
	})
	if err := service.refreshGroupMonitorSnapshotsAt(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&storage.RelayGroupMonitorDay{}).Where("day = ?", time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)).Update("updated_at", now.Add(-48*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := service.refreshGroupMonitorSnapshotsAt(context.Background(), now.Add(time.Minute)); err == nil {
		t.Fatal("expected history failure")
	}
	for _, preset := range []string{"today", "24h", "7d", "30d"} {
		row, err := service.stations.GetGroupMonitorSnapshot(context.Background(), station.ID, preset)
		if err != nil {
			t.Fatal(err)
		}
		if (row.LastError != "") != (preset == "30d") {
			t.Fatalf("%s wrong sync error: %s", preset, row.LastError)
		}
	}
}

func TestMonitorSnapshotFreshnessUsesConfiguredInterval(t *testing.T) {
	service, station, _ := monitorSnapshotFixture(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "not used", 500) })
	now := time.Now()
	group := PublicGroupMonitorGroup{ExternalID: 28, GroupStatus: "active", Status: "available", DataComplete: true}
	payload, _ := json.Marshal(PublicGroupMonitor{StationID: station.ID, Range: "24h", UpdatedAt: now.Add(-2 * time.Minute), SyncStatus: "ready", Groups: []PublicGroupMonitorGroup{group}})
	if err := service.stations.SaveGroupMonitorSnapshot(context.Background(), &storage.RelayGroupMonitorSnapshot{RelayStationID: station.ID, RangeName: "24h", SourceKey: monitorSourceKey(station), Payload: payload, UpdatedAt: now.Add(-2 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := service.settings.Set(storage.SettingGroupMonitorIntervalSeconds, "120"); err != nil {
		t.Fatal(err)
	}
	view, err := service.CachedPublicGroupMonitorForRange(context.Background(), station.ID, "24h")
	if err != nil || view.SyncStatus != "ready" {
		t.Fatalf("valid 120s interval marked stale: %+v %v", view, err)
	}
	if err := service.settings.Set(storage.SettingGroupMonitorIntervalSeconds, "10"); err != nil {
		t.Fatal(err)
	}
	view, err = service.CachedPublicGroupMonitorForRange(context.Background(), station.ID, "24h")
	if err != nil || view.SyncStatus != "stale" {
		t.Fatalf("late snapshot not marked stale: %+v %v", view, err)
	}
}
