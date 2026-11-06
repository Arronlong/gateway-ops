package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/monitor"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUsageCheckWindowPreservesHistoricalBoundsAndSkipsUnsettledCalls(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 13, 40, 0, time.UTC)
	since := now.Add(-time.Hour)
	start, end := usageCheckWindow(since, now, "24h", now)
	if !start.Equal(time.Date(2026, 11, 4, 11, 15, 0, 0, time.UTC)) || !end.Equal(time.Date(2026, 11, 4, 12, 10, 0, 0, time.UTC)) {
		t.Fatalf("window=%s/%s", start, end)
	}
	historicalEnd := now.Add(-24 * time.Hour)
	start, end = usageCheckWindow(since.Add(-48*time.Hour), historicalEnd, "custom", now)
	if !start.Equal(since.Add(-48*time.Hour)) || !end.Equal(historicalEnd) {
		t.Fatal("custom historical bounds were changed")
	}
}

func TestUsageIntegrityStreamFlushesProgressAndReturnsFinalResult(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "stream.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := db.DB()
	defer raw.Close()
	if err := db.AutoMigrate(&storage.Channel{}, &storage.ChannelAccount{}); err != nil {
		t.Fatal(err)
	}
	ch := storage.Channel{Name: "manual", Type: storage.ChannelTypeNewAPI, BalanceMode: storage.BalanceModeManual}
	if err := db.Create(&ch).Error; err != nil {
		t.Fatal(err)
	}
	channels := storage.NewChannels(db)
	deps := &Deps{Channels: channels, Monitor: monitor.NewService(channels, nil, nil, nil, nil, nil, nil)}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?range=custom&start=2026-10-01T00:00:00Z&end=2026-11-01T00:00:00Z&stream=1", nil)
	channelUsageIntegrity(ctx, deps)
	if recorder.Code != 200 || !strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") || !recorder.Flushed {
		t.Fatalf("stream was not flushed: status=%d headers=%v", recorder.Code, recorder.Header())
	}
	var stages []string
	for _, block := range strings.Split(recorder.Body.String(), "\n\n") {
		if !strings.HasPrefix(block, "data: ") {
			continue
		}
		var event struct {
			Stage string                 `json:"stage"`
			Data  monitor.UsageIntegrity `json:"data"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(block, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		stages = append(stages, event.Stage)
		if event.Stage == "done" && event.Data.Status != "not_applicable" {
			t.Fatalf("missing final result: %#v", event)
		}
	}
	if len(stages) != 2 || stages[0] != "usage" || stages[1] != "done" {
		t.Fatalf("stages=%v", stages)
	}
}

func TestUsageHistoryRejectsInvalidWindowBeforeStorageOrUpstreamAccess(t *testing.T) {
	for _, handler := range []func(*gin.Context, *Deps){channelUsageIntegrity, refetchChannelUsageHistory} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/?range=custom&start=bad&end=bad", nil)
		handler(ctx, &Deps{})
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d", recorder.Code)
		}
	}
}
