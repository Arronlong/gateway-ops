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
	_ "github.com/guliansheng/gateway-ops/internal/connector/newapi"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUsagePageCompleteRequiresShortPage(t *testing.T) {
	tests := []struct {
		name              string
		page              connector.UsagePage
		requestedPageSize int
		want              bool
	}{
		{
			name:              "full page overrides understated page count",
			page:              connector.UsagePage{RawCount: 100, PageSize: 100, Pages: 1},
			requestedPageSize: 1000,
			want:              false,
		},
		{
			name:              "full page uses requested size when response omits page size",
			page:              connector.UsagePage{RawCount: 100, Pages: 1},
			requestedPageSize: 100,
			want:              false,
		},
		{
			name:              "short page completes even when page count is overstated",
			page:              connector.UsagePage{RawCount: 35, PageSize: 100, Pages: 12},
			requestedPageSize: 100,
			want:              true,
		},
		{
			name:              "empty page completes",
			page:              connector.UsagePage{PageSize: 100, Pages: 12},
			requestedPageSize: 100,
			want:              true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usagePageComplete(&tt.page, tt.requestedPageSize); got != tt.want {
				t.Fatalf("usagePageComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelUsageSyncStartKeepsLegacyRepairSeparateFromIncrementalWindow(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	oldBackfill := now.Add(-20 * 24 * time.Hour)
	forward := now.Add(-time.Minute)
	state := &storage.ChannelUsageSyncState{BackfillVersion: 0, BackfilledAt: &oldBackfill, ForwardSyncedAt: &forward}

	got := channelUsageSyncStart(now, state, nil)
	want := forward.Add(-10 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("sync start = %s, want historical backfill start %s", got, want)
	}
}

func TestChannelUsageSyncStartUsesForwardCursorAfterBackfill(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	forward := now.Add(-time.Hour)
	state := &storage.ChannelUsageSyncState{BackfillVersion: channelUsageBackfillVersion, ForwardSyncedAt: &forward}

	got := channelUsageSyncStart(now, state, nil)
	want := forward.Add(-10 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("sync start = %s, want forward cursor overlap %s", got, want)
	}
}

func TestRefreshUsageRepairsLegacyBackfillAndKeepsAuditProgressOnFailure(t *testing.T) {
	for _, failSecondPage := range []bool{false, true} {
		t.Run(strconv.FormatBool(failSecondPage), func(t *testing.T) {
			now := time.Now().UTC()
			incremental := false
			var pages []int
			var requestStart time.Time
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/user/self":
					_, _ = w.Write([]byte(`{"success":true,"data":{"id":7}}`))
				case "/api/status":
					_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
				case "/api/log/self":
					page, _ := strconv.Atoi(r.URL.Query().Get("p"))
					unixStart, _ := strconv.ParseInt(r.URL.Query().Get("start_timestamp"), 10, 64)
					unixEnd, _ := strconv.ParseInt(r.URL.Query().Get("end_timestamp"), 10, 64)
					stamp := now.Add(-13 * 24 * time.Hour)
					if incremental {
						stamp = now.Add(-time.Minute)
					}
					inside := stamp.Unix() >= unixStart && stamp.Unix() <= unixEnd
					if r.URL.Query().Get("type") != "0" {
						total := 0
						items := []map[string]any{}
						if inside && r.URL.Query().Get("type") == "2" {
							total = 102
							if incremental {
								total = 1
							}
							items = append(items, map[string]any{"type": 2})
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": total, "page_size": 1, "items": items}})
						return
					}
					if inside {
						pages = append(pages, page)
						requestStart = time.Unix(unixStart, 0)
					}
					if page == 2 && failSecondPage {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"success":false,"message":"page failed"}`))
						return
					}
					count := 100
					if page == 2 {
						count = 2
					} else if page > 2 {
						count = 0
					}
					createdAt := now.Add(-13 * 24 * time.Hour)
					if incremental {
						count = 0
						if page == 1 {
							count = 1
						}
						createdAt = now.Add(-time.Minute)
					}
					if !inside {
						count = 0
					}
					items := make([]map[string]any, count)
					for i := range items {
						requestID := (page-1)*100 + i + 1
						if requestID == 101 {
							requestID = 100
						}
						if incremental {
							requestID = 1000 + i
						}
						items[i] = map[string]any{"id": (page-1)*100 + i + 1, "request_id": strconv.Itoa(requestID), "type": 2, "model_name": "model", "created_at": createdAt.Unix(), "quota": 500000}
					}
					total := 102
					if incremental {
						total = 1
					}
					if !inside {
						total = 0
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"items": items, "total": total, "page_size": 100, "page": page}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "usage.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&storage.Channel{}, &storage.ChannelAccount{}, &storage.ChannelUsageEvent{}, &storage.ChannelUsageSyncState{}, &storage.ChannelUsageHistorySeen{}); err != nil {
				t.Fatal(err)
			}
			cipher, _ := appcrypto.NewCipher("usage-pagination-test")
			credential, _ := cipher.Encrypt(`{"auth_type":"access_token","token":"test-token","user_id":"7"}`)
			ch := storage.Channel{Name: "test", Type: storage.ChannelTypeNewAPI, SiteURL: server.URL, Username: "monitor", PasswordCipher: credential, CredentialMode: storage.CredentialModeToken, BalanceMode: storage.BalanceModeAuto}
			if err := db.Create(&ch).Error; err != nil {
				t.Fatal(err)
			}
			account := storage.ChannelAccount{ChannelID: ch.ID, IsPrimary: true, Username: "monitor", PasswordCipher: credential, CredentialMode: storage.CredentialModeToken}
			if err := db.Create(&account).Error; err != nil {
				t.Fatal(err)
			}
			oldCursor := now.Add(-time.Hour)
			oldBackfill := now.Add(-time.Hour)
			usage := storage.NewChannelUsage(db)
			if err := usage.Upsert([]storage.ChannelUsageEvent{{ChannelID: ch.ID, ChannelAccountID: account.ID, Source: "newapi", RemoteUsageID: "1", ActualCost: 3, OccurredAt: now.Add(-13 * 24 * time.Hour), SyncedAt: now}}); err != nil {
				t.Fatal(err)
			}
			if err := usage.SaveSyncState(&storage.ChannelUsageSyncState{ChannelID: ch.ID, ChannelAccountID: account.ID, ForwardSyncedAt: &oldCursor, BackfilledAt: &oldBackfill}); err != nil {
				t.Fatal(err)
			}
			channels := storage.NewChannels(db)
			service := NewService(channels, nil, nil, usage, channel.NewService(channels, nil, nil, nil, cipher), nil, nil)
			service.usageLimiter = connector.NewUsageLimiter(0, time.Minute)
			err = service.RefreshUsage(context.Background(), &ch)
			if (err != nil) != failSecondPage {
				t.Fatalf("RefreshUsage() error = %v, expected failure %v", err, failSecondPage)
			}
			wantPages := 2
			if len(pages) != wantPages || pages[0] != 1 || pages[1] != 2 || requestStart.Unix() != now.Add(-13*24*time.Hour).Unix() {
				t.Fatalf("requested pages/start = %v/%s", pages, requestStart)
			}
			state, err := usage.SyncState(ch.ID, account.ID)
			if err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := db.Model(&storage.ChannelUsageEvent{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if failSecondPage {
				if state.BackfillVersion != 0 || state.HistoryCheckedAt != nil || state.HistoryVerifiedUntil == nil || state.HistoryScan == "" || state.ForwardSyncedAt == nil || state.ForwardSyncedAt.Before(oldCursor) || count != 101 {
					t.Fatalf("failed audit was marked complete or lost history: state=%#v count=%d", state, count)
				}
			} else if state.ForwardSyncedAt == nil || !state.ForwardSyncedAt.After(oldCursor) || count != 102 || state.HistoryScan == "" {
				t.Fatalf("successful sync state/count = %#v/%d", state, count)
			}
			if failSecondPage {
				failSecondPage = false
				if err := service.RefreshUsage(context.Background(), &ch); err != nil {
					t.Fatalf("backfill did not recover after retry: %v", err)
				}
			}
			incremental = true
			if err := service.RefreshUsage(context.Background(), &ch); err != nil {
				t.Fatal(err)
			}
			if requestStart.Before(now.Add(-11 * time.Minute)) {
				t.Fatalf("completed backfill fetched history again: %s", requestStart)
			}
			if err := db.Model(&storage.ChannelUsageEvent{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 103 {
				t.Fatalf("incremental sync overwrote history or lost identical requests: count=%d, want 103", count)
			}
		})
	}
}
