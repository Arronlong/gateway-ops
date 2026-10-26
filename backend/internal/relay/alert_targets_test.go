package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	appcrypto "github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAlertUserSnapshotsSupportsSub2APIAndNewAPI(t *testing.T) {
	for _, stationType := range []string{relayStationSub2API, relayStationNewAPI} {
		t.Run(stationType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stationType == relayStationNewAPI && (r.Header.Get("Authorization") != "Bearer admin-token" || r.Header.Get("New-Api-User") != "7") {
					t.Errorf("NewAPI authorization headers = %v", r.Header)
				}
				if stationType == relayStationNewAPI {
					switch r.URL.Path {
					case "/api/status":
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 100000}})
					case "/api/user/":
						if r.URL.Query().Get("page_size") != "100" {
							t.Errorf("NewAPI page size = %q", r.URL.Query().Get("page_size"))
						}
						data := map[string]any{"items": []any{map[string]any{"id": 42, "username": "active", "email": "active@example.com", "role": 1, "status": 1, "quota": 350000, "created_at": time.Now().Unix()}}, "total": 1, "page": 1, "page_size": 100}
						_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
					default:
						t.Errorf("unexpected NewAPI request: %s", r.URL.String())
						http.NotFound(w, r)
					}
					return
				}
				if r.URL.Path != "/api/v1/admin/users" || r.URL.Query().Get("page_size") == "" {
					t.Errorf("unexpected Sub2API user snapshot request: %s", r.URL.String())
				}
				page := remoteUserPage{Items: []remoteUser{
					{ID: 42, Username: "active", Email: "active@example.com", Role: "user", Balance: 3.5, Status: "active"},
					{ID: 43, Username: "deleted", DeletedAt: ptrTime(time.Now().UTC())},
				}, Total: 2, Page: 1, Pages: 1, PageSize: 500}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": page})
			}))
			defer server.Close()

			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alert-users.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			if err := db.AutoMigrate(&storage.RelayStation{}); err != nil {
				t.Fatal(err)
			}
			stations := storage.NewRelayStations(db)
			cipher, err := appcrypto.NewCipher("alert-user-test-secret")
			if err != nil {
				t.Fatal(err)
			}
			credential := "sub2api-admin-key"
			if stationType == relayStationNewAPI {
				credential, err = encodeRelayCredential(stationType, newAPICredential{AuthMode: "access_token", Token: "admin-token", UserID: "7"})
				if err != nil {
					t.Fatal(err)
				}
			}
			ciphertext, err := cipher.Encrypt(credential)
			if err != nil {
				t.Fatal(err)
			}
			station := storage.RelayStation{Name: "alert target", Type: stationType, BaseURL: server.URL, APIKeyCipher: ciphertext}
			if err := stations.Create(&station); err != nil {
				t.Fatal(err)
			}

			users, err := NewService(stations, cipher).AlertUserSnapshots(context.Background(), station.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(users) != 1 || users[0].ID != 42 || users[0].Balance != 3.5 || users[0].ObservedAt.IsZero() {
				t.Fatalf("user snapshots = %+v", users)
			}
		})
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
