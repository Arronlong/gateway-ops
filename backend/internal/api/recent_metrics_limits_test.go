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
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRecentLatencyTrendsHonorThousandSamples(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "recent-metrics.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.Channel{}, &storage.ChannelUsageEvent{}, &storage.RelayStation{}, &storage.RelayAccount{}, &storage.RelayUsageEvent{}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{&storage.Channel{ID: 1, Name: "channel"}, &storage.RelayStation{ID: 1, Name: "station"}, &storage.RelayAccount{RelayStationID: 1, ExternalID: 3, Name: "account"}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	duration := int64(1000)
	channelRows := make([]storage.ChannelUsageEvent, 1001)
	relayRows := make([]storage.RelayUsageEvent, 1001)
	for i := range channelRows {
		at := now.Add(-time.Duration(i+1) * 6 * time.Minute)
		channelRows[i] = storage.ChannelUsageEvent{ChannelID: 1, Source: "fixture", RemoteUsageID: fmt.Sprint(i + 1), Model: "text", DurationMS: &duration, InputTokens: 10, OutputTokens: 40, OccurredAt: at}
		relayRows[i] = storage.RelayUsageEvent{RelayStationID: 1, AccountExternalID: 3, RemoteUsageID: int64(i + 1), Model: "text", FirstTokenMS: 100, DurationMS: 1000, InputTokens: 10, OutputTokens: 40, StatusCode: 200, OccurredAt: at}
	}
	if err := db.CreateInBatches(channelRows, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInBatches(relayRows, 100).Error; err != nil {
		t.Fatal(err)
	}
	stations := storage.NewRelayStations(db)
	deps := &Deps{Channels: storage.NewChannels(db), ChannelUsage: storage.NewChannelUsage(db), RelayStations: stations, Relay: relay.NewService(stations, nil, nil, nil, nil)}
	router := gin.New()
	router.GET("/channels/latency-trends", func(c *gin.Context) { channelLatencyTrends(c, deps) })
	router.GET("/relay-stations/latency-trends", func(c *gin.Context) { relayStationLatencyTrends(c, deps) })
	for _, endpoint := range []string{"/channels/latency-trends", "/relay-stations/latency-trends"} {
		for _, limit := range []int{1, 201, 1000} {
			t.Run(fmt.Sprintf("%s/%d", endpoint, limit), func(t *testing.T) {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("%s?limit=%d", endpoint, limit), nil))
				if response.Code != 200 {
					t.Fatalf("status=%d: %s", response.Code, response.Body.String())
				}
				var payload struct {
					Data []struct {
						Samples []json.RawMessage `json:"samples"`
					} `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if len(payload.Data) != 1 || len(payload.Data[0].Samples) != limit {
					t.Fatalf("expected %d samples; got %d series: %s", limit, len(payload.Data), response.Body.String()[:min(300, response.Body.Len())])
				}
			})
		}
	}
}
