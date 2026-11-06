package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/channel"
	"github.com/guliansheng/gateway-ops/internal/connector"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestManualUsageHistoryRepairsCountsWithoutMovingIncrementalCursor(t *testing.T) {
	for _, failRead := range []bool{false, true} {
		t.Run(strconv.FormatBool(failRead), func(t *testing.T) {
			now := time.Now().UTC().AddDate(0, 0, -1).Truncate(24 * time.Hour).Add(12 * time.Hour)
			since := now.Add(-3 * time.Hour)
			until := now.Add(-time.Hour)
			stamp := now.Add(-2 * time.Hour).Truncate(time.Second)
			countRequests := 0
			countsAvailable, emptyHistory := true, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/user/self":
					_, _ = w.Write([]byte(`{"success":true,"data":{"id":7}}`))
				case "/api/status":
					_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
				case "/api/log/self":
					kind := r.URL.Query().Get("type")
					if kind != "0" {
						countRequests++
						if !countsAvailable {
							_, _ = w.Write([]byte(`{"success":true,"data":{"items":[]}}`))
							return
						}
						total := 2
						items := []map[string]any{{"type": 2}}
						if kind == "5" {
							total = 0
							items = nil
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": total, "page_size": 1, "items": items}})
						return
					}
					if failRead {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					items := []map[string]any{{"id": 1, "type": 2, "request_id": "a", "model_name": "model", "created_at": stamp.Unix(), "quota": 500000}, {"id": 2, "type": 2, "request_id": "b", "model_name": "model", "created_at": stamp.Unix(), "quota": 1000000}}
					if emptyHistory {
						items = nil
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": 2, "page_size": 100, "items": items}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "audit.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&storage.Channel{}, &storage.ChannelAccount{}, &storage.ChannelUsageEvent{}, &storage.ChannelUsageSyncState{}, &storage.ChannelUsageHistorySeen{}); err != nil {
				t.Fatal(err)
			}
			cipher, _ := appcrypto.NewCipher("audit-test")
			credential, _ := cipher.Encrypt(`{"auth_type":"access_token","token":"test","user_id":"7"}`)
			ch := storage.Channel{Name: "test", Type: storage.ChannelTypeNewAPI, SiteURL: server.URL, Username: "test", PasswordCipher: credential, CredentialMode: storage.CredentialModeToken, BalanceMode: storage.BalanceModeAuto}
			if err := db.Create(&ch).Error; err != nil {
				t.Fatal(err)
			}
			account := storage.ChannelAccount{ChannelID: ch.ID, IsPrimary: true, Username: "test", PasswordCipher: credential, CredentialMode: storage.CredentialModeToken}
			if err := db.Create(&account).Error; err != nil {
				t.Fatal(err)
			}
			repo := storage.NewChannelUsage(db)
			cursor := now.Add(-10 * time.Minute)
			if err := repo.SaveSyncState(&storage.ChannelUsageSyncState{ChannelID: ch.ID, ChannelAccountID: account.ID, BackfillVersion: channelUsageBackfillVersion, ForwardSyncedAt: &cursor}); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if err := repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: ch.ID, ChannelAccountID: account.ID, Source: "newapi", RemoteUsageID: "newapi:v2:stale-" + strconv.Itoa(i), ActualCost: 20, OccurredAt: stamp, SyncedAt: now}}); err != nil {
					t.Fatal(err)
				}
			}
			channels := storage.NewChannels(db)
			svc := NewService(channels, nil, nil, repo, channel.NewService(channels, nil, nil, nil, cipher), nil, nil)
			svc.usageLimiter = connector.NewUsageLimiter(0, time.Minute)
			check, err := svc.CheckUsageIntegrity(context.Background(), &ch, since, until)
			if err != nil || check.Status != "mismatch" || check.LocalCount != 3 || check.UpstreamCount == nil || *check.UpstreamCount != 2 {
				t.Fatalf("before: %#v error=%v", check, err)
			}
			_, _ = svc.CheckUsageIntegrity(context.Background(), &ch, since, until)
			if countRequests != 2 {
				t.Fatalf("upstream count was not cached: %d", countRequests)
			}
			err = svc.RefetchUsageHistory(context.Background(), &ch, since, until)
			if (err != nil) != failRead {
				t.Fatalf("refetch error=%v", err)
			}
			state, _ := repo.SyncState(ch.ID, account.ID)
			if state.ForwardSyncedAt == nil || !state.ForwardSyncedAt.Equal(cursor) || state.BackfillVersion != channelUsageBackfillVersion {
				t.Fatalf("manual refetch changed automatic cursor: %#v", state)
			}
			events, _ := repo.EventsBetween(ch.ID, since, until)
			if failRead {
				if len(events) != 3 {
					t.Fatalf("failed refetch changed data: %d", len(events))
				}
				return
			}
			if len(events) != 2 || events[0].ActualCost+events[1].ActualCost != 3 {
				t.Fatalf("refetch did not replace stale records: %#v", events)
			}
			check, err = svc.CheckUsageIntegrity(context.Background(), &ch, since, until)
			if err != nil || check.Status != "matched" || check.LocalCount != 2 {
				t.Fatalf("after: %#v error=%v", check, err)
			}
			emptyHistory = true
			if err := repo.Upsert([]storage.ChannelUsageEvent{{ChannelID: ch.ID, ChannelAccountID: account.ID, Source: "newapi", RemoteUsageID: "newapi:v2:extra-stale", OccurredAt: stamp, SyncedAt: now}}); err != nil {
				t.Fatal(err)
			}
			if err := svc.RefetchUsageHistory(context.Background(), &ch, since, until); err == nil {
				t.Fatal("empty upstream history was silently accepted over existing records")
			}
			events, _ = repo.EventsBetween(ch.ID, since, until)
			if len(events) != 3 {
				t.Fatal("empty upstream response deleted existing history")
			}
			countsAvailable = false
			check, err = svc.CheckUsageIntegrity(context.Background(), &ch, since, until)
			if err != nil || check.Status != "unavailable" || check.UpstreamCount != nil {
				t.Fatalf("unknown total was reported as healthy: %#v error=%v", check, err)
			}
		})
	}
}
