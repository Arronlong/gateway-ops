package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func recentMetricsSyncFixture(t *testing.T, limit int) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "metrics-sync.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.AppSetting{}, &storage.RelayAccount{}, &storage.RelayUsageEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.RelayAccount{RelayStationID: 1, ExternalID: 11, Name: "account", Platform: "openai"}).Error; err != nil {
		t.Fatal(err)
	}
	settings := storage.NewSettings(db)
	if limit > 0 {
		if err := settings.Set(storage.SettingUIRecentMetricsLimit, strconv.Itoa(limit)); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(storage.NewRelayStations(db), nil, nil, nil, settings)
}

func TestRecentAccountUsageCollectionFollowsMetricsLimit(t *testing.T) {
	for _, tc := range []struct {
		name         string
		limit        int
		upstreamSize int
		total        int
		onlyTotal    bool
		failPage     int
		repeatPage   bool
		want         int
		wantRequests int32
	}{
		{name: "default", upstreamSize: 200, total: 1100, want: 60, wantRequests: 1},
		{name: "minimum", limit: 1, upstreamSize: 200, total: 1100, want: 1, wantRequests: 1},
		{name: "two-hundred", limit: 200, upstreamSize: 200, total: 1100, want: 200, wantRequests: 1},
		{name: "capped-page", limit: 200, upstreamSize: 60, total: 1100, want: 200, wantRequests: 4},
		{name: "capped-page-with-total-only", limit: 1000, upstreamSize: 60, total: 1100, onlyTotal: true, want: 1000, wantRequests: 17},
		{name: "thousand", limit: 1000, upstreamSize: 200, total: 1100, want: 1000, wantRequests: 5},
		{name: "less-than-limit", limit: 200, upstreamSize: 60, total: 73, want: 73, wantRequests: 2},
		{name: "retain-pages-on-failure", limit: 200, upstreamSize: 60, total: 1100, failPage: 2, want: 60, wantRequests: 2},
		{name: "ignore-repeated-pages", limit: 200, upstreamSize: 60, total: 1100, repeatPage: true, want: 60, wantRequests: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := recentMetricsSyncFixture(t, tc.limit)
			now := time.Now().UTC().Truncate(time.Second)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/api/v1/admin/usage" || r.URL.Query().Get("account_id") != "11" {
					t.Errorf("unexpected request: %s", r.URL.String())
					http.NotFound(w, r)
					return
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				requestedSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
				limit := tc.limit
				if limit == 0 {
					limit = 60
				}
				if requestedSize != min(limit, 200) || page < 1 {
					t.Errorf("page=%d size=%d, want size=%d", page, requestedSize, min(limit, 200))
				}
				if page == tc.failPage {
					http.Error(w, "rate limited", http.StatusTooManyRequests)
					return
				}
				size := min(requestedSize, tc.upstreamSize)
				start := (page - 1) * size
				if tc.repeatPage {
					start = 0
				}
				items := make([]remoteUsage, 0, size)
				for i := start; i < min(start+size, tc.total); i++ {
					items = append(items, remoteUsage{ID: int64(tc.total - i), AccountID: 11, GroupID: 28,
						CreatedAt: now.Add(-time.Duration(i+2) * time.Minute).Format(time.RFC3339Nano),
						Model:     "text", FirstTokenMS: 100, DurationMS: 1000, InputTokens: 10, CacheReadTokens: 10, OutputTokens: 40, StatusCode: 200})
				}
				response := remoteUsagePage{Items: items, Page: page, PageSize: size, Total: tc.total, Pages: (tc.total + size - 1) / size}
				if tc.onlyTotal {
					response.PageSize, response.Pages = 0, 0
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": response})
			}))
			defer server.Close()
			service.client = server.Client()
			accounts := []remoteAccount{{ID: 11, Type: "apikey", Status: "active", Schedulable: true, Platform: "openai"}}
			items := service.fetchRecentUsageItemsForAccounts(context.Background(), server.URL, "key", accounts)
			if len(items) != tc.want || requests.Load() != tc.wantRequests {
				t.Fatalf("collected=%d requests=%d, want %d/%d", len(items), requests.Load(), tc.want, tc.wantRequests)
			}
			if err := service.persistUsageItems(1, items, now); err != nil {
				t.Fatal(err)
			}
			snapshots := service.localAccountLatencySnapshots(1, accounts, []remoteGroup{{ID: 28, RateMultiplier: 1.25}})
			var saved []storage.RelayLatencySample
			if err := json.Unmarshal([]byte(snapshots[11]), &saved); err != nil {
				t.Fatal(err)
			}
			limit := tc.limit
			if limit == 0 {
				limit = 60
			}
			trends, err := service.RecentAccountLatencySamples(1, limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(saved) != tc.want || len(trends[11]) != tc.want {
				t.Fatalf("snapshot=%d trends=%d, want %d", len(saved), len(trends[11]), tc.want)
			}
			if saved[0].Platform != "openai" || saved[0].GroupMultiplier == nil || *saved[0].GroupMultiplier != 1.25 {
				t.Fatalf("snapshot lost account or group metadata: %+v", saved[0])
			}
		})
	}
}

func TestRecentAccountSnapshotUsesConfiguredLimit(t *testing.T) {
	for _, limit := range []int{1, 60, 200, 1000} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			service := recentMetricsSyncFixture(t, limit)
			now := time.Now().UTC()
			items := make([]remoteUsage, 1001)
			for i := range items {
				items[i] = remoteUsage{ID: int64(i + 1), AccountID: 11, CreatedAt: now.Add(-time.Duration(i+1) * time.Minute).Format(time.RFC3339Nano)}
			}
			if err := service.persistUsageItems(1, items, now); err != nil {
				t.Fatal(err)
			}
			snapshots := service.localAccountLatencySnapshots(1, []remoteAccount{{ID: 11}}, nil)
			var samples []storage.RelayLatencySample
			if err := json.Unmarshal([]byte(snapshots[11]), &samples); err != nil {
				t.Fatal(err)
			}
			if len(samples) != limit {
				t.Fatalf("snapshot kept %d samples, want %d", len(samples), limit)
			}
		})
	}
}
