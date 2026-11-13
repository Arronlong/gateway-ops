package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/channel"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func channelStatisticsRouter(t *testing.T) (*gin.Engine, *gorm.DB, *Deps) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "statistics.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.Channel{}, &storage.ChannelAccount{}, &storage.ChannelRelayAccountLink{},
		&storage.ChannelUsageEvent{}, &storage.BalanceChangeLog{}, &storage.RateSnapshot{}, &storage.RateChangeLog{},
		&storage.OperationLedgerEntry{}, &storage.RelayStation{}, &storage.NotificationLog{}); err != nil {
		t.Fatal(err)
	}
	channels := storage.NewChannels(db)
	stations := storage.NewRelayStations(db)
	d := &Deps{Channels: channels, ChannelUsage: storage.NewChannelUsage(db), Rates: storage.NewRates(db),
		Operations: storage.NewOperations(db), RelayStations: stations, Relay: relay.NewService(stations, nil),
		ChannelSvc: channel.NewService(channels, storage.NewAuthSessions(db), nil, nil, nil), Notifies: storage.NewNotifications(db)}
	router := gin.New()
	registerChannels(router.Group(""), d)
	registerDashboard(router.Group(""), d)
	return router, db, d
}

func TestChannelStatisticsDefaultsAndIndependentUpdates(t *testing.T) {
	router, _, d := channelStatisticsRouter(t)
	for _, excluded := range []bool{false, true} {
		body := map[string]any{"name": fmt.Sprint("channel-", excluded), "type": "newapi", "site_url": "https://example.invalid", "balance_mode": "manual", "monitor_enabled": true}
		if excluded {
			body["exclude_from_statistics"] = true
		}
		data := brandingRequest(t, router, "POST", "/channels", body, 200)
		if data["exclude_from_statistics"] != excluded {
			t.Fatalf("create: %v", data)
		}
		id := uint(data["id"].(float64))
		path := fmt.Sprintf("/channels/%d", id)
		data = brandingRequest(t, router, "PUT", path, map[string]any{"remark": "keep statistics choice"}, 200)
		if data["exclude_from_statistics"] != excluded {
			t.Fatalf("partial update reset choice: %v", data)
		}
		for _, next := range []bool{true, false} {
			data = brandingRequest(t, router, "PUT", path, map[string]any{"exclude_from_statistics": next}, 200)
			if data["exclude_from_statistics"] != next || data["monitor_enabled"] != true {
				t.Fatalf("toggle: %v", data)
			}
		}
		ch, err := d.Channels.FindByID(id)
		if err != nil || ch.Remark != "keep statistics choice" {
			t.Fatalf("saved=%+v err=%v", ch, err)
		}
	}
}

func TestChannelSummariesExcludeDuplicatesAndRetainDetails(t *testing.T) {
	router, db, _ := channelStatisticsRouter(t)
	for i := 0; i < 3; i++ {
		data := brandingRequest(t, router, "POST", "/channels", map[string]any{"name": fmt.Sprint("shared-", i), "type": "newapi", "site_url": "https://example.invalid", "balance_mode": "manual", "manual_balance": 100, "exclude_from_statistics": i != 0, "monitor_enabled": true}, 200)
		id := uint(data["id"].(float64))
		if err := db.Model(&storage.Channel{}).Where("id = ?", id).Updates(map[string]any{"balance_mode": "auto", "monitor_enabled": i != 0}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&storage.BalanceChangeLog{ChannelID: id, BalanceSnapshotID: id, Delta: -40, PreviousBalance: 140, NewBalance: 100, DetectedAt: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&storage.ChannelUsageEvent{ChannelID: id, RemoteUsageID: "same-call", Source: "newapi", ActualCost: 40, OccurredAt: time.Now().UTC().Add(-time.Minute)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range []string{"/channels/summary?range=all", "/dashboard/summary?range=all"} {
		data := brandingRequest(t, router, "GET", endpoint, nil, 200)
		if data["total_balance"] != float64(100) || data["consumption_amount"] != float64(40) || data["total_channels"] != float64(3) {
			t.Fatalf("%s: %v", endpoint, data)
		}
		if endpoint == "/channels/summary?range=all" && (data["usage_cost_amount"] != float64(40) || data["usage_request_count"] != float64(1)) {
			t.Fatalf("usage still duplicated: %v", data)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/channels/metrics?range=all", nil))
	if response.Code != 200 {
		t.Fatalf("details: %s", response.Body.String())
	}
	var details struct {
		Data []struct {
			UsageCost float64 `json:"usage_cost_amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &details); err != nil {
		t.Fatal(err)
	}
	if len(details.Data) != 3 {
		t.Fatalf("missing detail rows: %s", response.Body.String())
	}
	for _, detail := range details.Data {
		if detail.UsageCost != 40 {
			t.Fatalf("excluded channel lost detail: %+v", detail)
		}
	}
	var count int64
	db.Model(&storage.ChannelUsageEvent{}).Count(&count)
	if count != 3 {
		t.Fatalf("excluding statistics must retain all history: %d", count)
	}
	brandingRequest(t, router, "PUT", "/channels/2", map[string]any{"exclude_from_statistics": false}, 200)
	data := brandingRequest(t, router, "GET", "/channels/summary?range=all", nil, 200)
	if data["total_balance"] != float64(200) || data["usage_cost_amount"] != float64(80) {
		t.Fatalf("re-enabled history was not counted: %v", data)
	}
}

func TestOperationStatisticsExclusionDoesNotRemoveLedgerExpenses(t *testing.T) {
	_, db, d := channelStatisticsRouter(t)
	balance := 100.0
	channels := []storage.Channel{{ID: 1, LastBalance: &balance}, {ID: 2, LastBalance: &balance, ExcludeFromStatistics: true}, {ID: 3, BalanceMode: storage.BalanceModeManual, ManualBalance: 100, ExcludeFromStatistics: true}}
	amount, decrease := operationChannelBalances(channels, map[uint]float64{1: 40, 2: 40, 3: 40})
	if amount != 100 || decrease != 40 {
		t.Fatalf("balance=%v decrease=%v", amount, decrease)
	}
	if cost := operationActualCost(0, 200, amount); cost != 100 {
		t.Fatalf("duplicated balance distorted actual cost: %v", cost)
	}
	id := uint(2)
	if err := db.Create(&storage.OperationLedgerEntry{ChannelID: &id, Direction: "expense", Category: "upstream_recharge", Amount: 200, OccurredAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	ledger, _, err := d.Operations.LedgerSummary(time.Time{})
	if err != nil || ledger.ExpenseAmount != 200 || ledger.UpstreamRechargeAmount != 200 {
		t.Fatalf("ledger=%+v err=%v", ledger, err)
	}
}
