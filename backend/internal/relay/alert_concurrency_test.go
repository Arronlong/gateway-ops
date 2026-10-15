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
	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestManualSchedulingWaitsForAlertEvaluation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "manual.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.RelayStation{}, &storage.RelayAccount{}, &storage.AlertRuleState{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.RelayStation{ID: 7, Name: "测试站点", BaseURL: "http://127.0.0.1:9"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.RelayAccount{RelayStationID: 7, ExternalID: 42, Schedulable: false}).Error; err != nil {
		t.Fatal(err)
	}
	repo := storage.NewAlerts(db)
	state, err := repo.GetOrCreateState(storage.AlertStateKey{RuleID: 1, TargetType: "relay_account", RelayStationID: 7, TargetExternalID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(storage.NewRelayStations(db), nil)
	acquired, release, evaluated := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		evaluated <- repo.WithEvaluationLock(context.Background(), func(locked *storage.Alerts) error {
			close(acquired)
			<-release
			return locked.SaveState(state)
		})
	}()
	<-acquired
	manual := make(chan error, 1)
	go func() { manual <- service.SetSchedulable(context.Background(), 7, 42, false) }()
	select {
	case err := <-manual:
		close(release)
		<-evaluated
		t.Fatalf("manual mutation bypassed the evaluation lock: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-evaluated; err != nil {
		t.Fatal(err)
	}
	if err := <-manual; err != nil {
		t.Fatal(err)
	}
	var saved storage.AlertRuleState
	if err := db.First(&saved, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !saved.ManualIntervened {
		t.Fatal("evaluation overwrote the manual intervention marker")
	}
}

func TestSub2APIAutomationAdapterReadsAndWritesRealState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "adapter.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.RelayStation{}, &storage.RelayAccount{}, &storage.RelayGroup{}, &storage.RelayAccountGroup{}, &storage.RelayAccountAdjustmentLog{}, &storage.AlertRuleState{}); err != nil {
		t.Fatal(err)
	}
	accountEnabled, userStatus := true, "active"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-admin-key" {
			t.Error("missing upstream authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/admin/accounts/42":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"schedulable": accountEnabled, "updated_at": "account-version"}})
		case "POST /api/v1/admin/accounts/42/schedulable":
			var body struct {
				Enabled bool `json:"schedulable"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			accountEnabled = body.Enabled
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		case "GET /api/v1/admin/users/99":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"status": userStatus, "updated_at": "user-version"}})
		case "PUT /api/v1/admin/users/99":
			var body struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			userStatus = body.Status
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		default:
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cipher, _ := crypto.NewCipher("adapter-test-secret")
	secret, _ := cipher.Encrypt("test-admin-key")
	if err := db.Create(&storage.RelayStation{ID: 7, Name: "协议测试", BaseURL: server.URL, APIKeyCipher: secret}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&storage.RelayAccount{RelayStationID: 7, ExternalID: 42, Schedulable: true}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(storage.NewRelayStations(db), cipher)
	for _, target := range []struct{ kind, id string }{{"relay_account", "42"}, {"relay_user", "99"}} {
		enabled, version, err := svc.ReadAutomationState(context.Background(), 7, target.kind, target.id)
		if err != nil || !enabled || version == "" {
			t.Fatalf("read %s = %v %s %v", target.kind, enabled, version, err)
		}
		if err := svc.SetAutomationState(context.Background(), 7, target.kind, target.id, false); err != nil {
			t.Fatal(err)
		}
		enabled, _, err = svc.ReadAutomationState(context.Background(), 7, target.kind, target.id)
		if err != nil || enabled {
			t.Fatalf("disabled state not observed for %s: %v", target.kind, err)
		}
		if err := svc.SetAutomationState(context.Background(), 7, target.kind, target.id, true); err != nil {
			t.Fatal(err)
		}
	}
	if !accountEnabled || userStatus != "active" {
		t.Fatal("enable actions were not mapped correctly")
	}
	if _, _, err := svc.ReadAutomationState(context.Background(), 7, "relay_user", "person@example.com"); err == nil {
		t.Fatal("email accepted as automation ID")
	}
}
