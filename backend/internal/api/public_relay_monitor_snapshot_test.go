package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPublicGroupMonitorSnapshotAPIReturnsColdRangesWithoutUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "slow upstream", http.StatusGatewayTimeout)
	}))
	defer upstream.Close()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "public.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&storage.RelayStation{}, &storage.RelayGroup{}, &storage.RelayGroupMonitorSnapshot{}); err != nil {
		t.Fatal(err)
	}
	repo := storage.NewRelayStations(db)
	station := storage.RelayStation{Name: "station", Type: "sub2api", BaseURL: upstream.URL, APIKeyCipher: "unused"}
	if err := repo.Create(&station); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.RelayGroup{RelayStationID: station.ID, ExternalID: 2, Name: "group", Status: "active", MonitorEnabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	deps := &Deps{Relay: relay.NewService(repo, nil), RelayStations: repo}
	router := gin.New()
	router.GET("/relay-stations/:id/group-monitor", func(c *gin.Context) { publicRelayGroupMonitor(c, deps) })
	for _, preset := range []string{"today", "24h", "7d", "30d"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/relay-stations/%d/group-monitor?range=%s", station.ID, preset), nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d %s", preset, response.Code, response.Body.String())
		}
		var body struct {
			Data relay.PublicGroupMonitor `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Data.Range != preset || body.Data.SyncStatus != "preparing" || len(body.Data.Groups) != 1 {
			t.Fatalf("%s body=%+v", preset, body)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("public API made %d upstream requests", requests.Load())
	}
}
