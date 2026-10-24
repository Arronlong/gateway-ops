package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestGroupPelicanRejectsUnboundAdministratorKey(t *testing.T) {
	id := int64(20)
	other := int64(21)
	items := []remoteGroupAPIKey{{Key: "wrong-group", Status: "active", GroupID: &other, User: &remoteUser{Role: "admin"}}, {Key: "unbound", Status: "active", User: &remoteUser{Role: "admin"}}}
	if got := selectAdminGroupAPIKey(items, id, true); got != "" {
		t.Fatalf("must not use an unbound or different-group key: %s", got)
	}
	items = append(items, remoteGroupAPIKey{Key: "correct", Status: "active", GroupID: &id, User: &remoteUser{Role: "admin"}})
	if got := selectAdminGroupAPIKey(items, id, true); got != "correct" {
		t.Fatalf("exact key not selected: %s", got)
	}
}

func TestGroupPelicanExecutesGlobalPlanThroughGroupGateway(t *testing.T) {
	var generations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/groups/20/api-keys":
			if r.Header.Get("x-api-key") != "station-admin-secret" {
				t.Error("missing management auth")
			}
			w.Write([]byte(`{"code":0,"data":{"items":[{"key":"unbound","status":"active","user":{"role":"admin"}},{"key":"group-key","group_id":20,"status":"active","user":{"role":"admin"}}],"pages":1}}`))
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer group-key" {
				t.Error("wrong model discovery key")
			}
			w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
		case "/v1/chat/completions":
			generations.Add(1)
			if r.Header.Get("Authorization") != "Bearer group-key" || r.Header.Get("x-api-key") != "" {
				t.Error("generation must not use management key")
			}
			var request struct {
				Model    string `json:"model"`
				Effort   string `json:"reasoning_effort"`
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Model != "model-a" || request.Effort != "high" || len(request.Messages) != 1 || !strings.HasSuffix(request.Messages[0].Content, pelicanContract) {
				t.Errorf("plan or HTML contract missing: %#v", request)
			}
			w.Write([]byte(`{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`))
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	service, mock := pelicanExecutionDB(t)
	mock.MatchExpectationsInOrder(false)
	service.cipher, _ = crypto.NewCipher("group-pelican-test-secret")
	cipher, _ := service.cipher.Encrypt("station-admin-secret")
	for i := 0; i < 2; i++ {
		mock.ExpectQuery(`SELECT .*relay_groups`).WillReturnRows(sqlmock.NewRows([]string{"relay_station_id", "external_id", "name", "rate_multiplier"}).AddRow(7, 20, "指定销售组", 0.5))
	}
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "type", "base_url", "api_key_cipher"}).AddRow(7, "sub2api", server.URL, cipher))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_group_pelican_results"`).WithArgs(uint(7), int64(20), "指定销售组", "model-a", "draw", "high", "manual", "success", "<svg></svg>", "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "指定销售组", 0.5).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
	mock.ExpectQuery(`SELECT .*app_settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(storage.SettingPelicanHistoryCount, "30"))
	mock.ExpectExec(`DELETE FROM "relay_group_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_group_pelican_configs".*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	service.runGroupPelicanBatch(context.Background(), storage.GroupPelicanConfig{ID: 1, RelayStationID: 7, GroupExternalID: 20}, time.Now().Add(time.Minute), time.Time{}, "manual", []storage.PelicanTestPlan{{Model: "missing-model,model-a", Prompt: "draw", ReasoningEffort: "high"}})
	if generations.Load() != 1 {
		t.Fatalf("generations=%d", generations.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGroupPelicanDisabledTasksDoesNotQueryOrClaimTargets(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	expectGlobalPelicanWindow(mock, "model-a", "00:00", "00:00", false)
	if _, err := service.StartGroupPelican(context.Background(), 7, 20); err == nil {
		t.Fatal("disabled tasks must reject manual generation")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
