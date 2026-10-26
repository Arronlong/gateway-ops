package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/alert"
	"github.com/guliansheng/gateway-ops/internal/auth"
	"github.com/guliansheng/gateway-ops/internal/config"
	"github.com/guliansheng/gateway-ops/internal/scheduler"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAlertRuleFromInputRejectsInvalidJSON(t *testing.T) {
	_, err := alertRuleFromInput(0, alertRuleInput{
		Name: "test", TargetType: alert.TargetRelayAccount, Metric: alert.MetricRateChange,
		Condition: json.RawMessage(`{"threshold":`),
	})
	if err == nil {
		t.Fatal("expected invalid JSON to be rejected")
	}
}

func TestAlertRuleFromInputUsesDefaults(t *testing.T) {
	rule, err := alertRuleFromInput(0, alertRuleInput{Name: "test", TargetType: alert.TargetRelayAccount, Metric: alert.MetricRateChange})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Severity != "warning" || rule.ConditionJSON != "{}" || !rule.Enabled {
		t.Fatalf("rule defaults = %+v", rule)
	}
}

type previewOnlyCollector struct{}

func (previewOnlyCollector) Collect(context.Context, storage.AlertRule, alert.ConditionConfig, time.Time) ([]alert.TargetObservation, error) {
	return []alert.TargetObservation{{StationID: 7, TargetExternalID: "42", TargetName: "测试账号", Observation: alert.Observation{ObservedAt: time.Now().UTC(), Value: 30, RequestCount: 10, Valid: true}}}, nil
}

func TestAlertHTTPWorkflowAndAuthentication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "api.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&storage.AlertRule{}, &storage.AlertRuleState{}, &storage.AlertEvent{}, &storage.AlertActionLog{}, &storage.AlertMetricSample{}, &storage.AlertRuleNotificationChannel{}, &storage.NotificationChannel{}, &storage.NotificationLog{}, &storage.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	repo := storage.NewAlerts(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := alert.NewService(repo, previewOnlyCollector{}, nil, nil, log)
	notifies, settings := storage.NewNotifications(db), storage.NewSettings(db)
	channel := storage.NotificationChannel{Name: "运维渠道", Type: storage.NotifyWebhook, Enabled: true}
	if err := notifies.CreateChannel(&channel); err != nil {
		t.Fatal(err)
	}
	authSvc, err := auth.New("tester", "password-for-test", "alert-api-test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authSvc.Login("tester", "password-for-test")
	if err != nil {
		t.Fatal(err)
	}
	sch := scheduler.New(config.SchedulerConfig{}, nil, nil, nil, notifies, settings, nil, svc, log)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &Deps{DB: db, Auth: authSvc, Alerts: svc, Notifies: notifies, Settings: settings, Scheduler: sch, Log: log})
	request := func(method, path, body, bearer string, expected int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, "/api"+path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != expected {
			t.Fatalf("%s %s = %d, want %d: %s", method, path, res.Code, expected, res.Body.String())
		}
		return res.Body.Bytes()
	}
	for _, path := range []string{"/alerts/rules", "/alerts/events", "/alerts/summary", "/alerts/settings"} {
		request("GET", path, "", "", 401)
	}
	request("POST", "/alerts/rules", `{}`, "", 401)
	body := fmt.Sprintf(`{"name":"消耗保护","target_type":"relay_account","metric":"account_spend","scope":{"station_ids":[7],"target_ids":["42"]},"condition":{"threshold":20,"window_seconds":900,"trigger_consecutive":3,"recovery_consecutive":3},"action":{"actions":[]},"recovery":{"enabled":false},"channel_ids":[%d],"cooldown_seconds":1800}`, channel.ID)
	created := request("POST", "/alerts/rules", body, token, 200)
	request("POST", "/alerts/rules/preview", body, token, 200)
	var result struct {
		Data storage.AlertRule `json:"data"`
	}
	if err := json.Unmarshal(created, &result); err != nil {
		t.Fatal(err)
	}
	ruleID := result.Data.ID
	if ruleID == 0 || len(result.Data.ChannelIDs) != 1 {
		t.Fatalf("created rule linkage: %s", created)
	}
	path := fmt.Sprintf("/alerts/rules/%d", ruleID)
	request("GET", path, "", token, 200)
	channelBody := `{"name":"渠道余额","target_type":"channel","metric":"balance_low","severity":"critical","enabled":true,"scope":{"logic":"all","filters":[{"field":"monitor_enabled","operator":"eq","value":"true"}]},"condition":{"threshold":0,"window_seconds":900},"action":{"actions":[]},"recovery":{"enabled":false},"cooldown_seconds":1800}`
	channelRule := request("POST", "/alerts/rules", channelBody, token, 200)
	if !bytes.Contains(channelRule, []byte(`"target_type":"channel"`)) {
		t.Fatalf("channel rule type was not persisted: %s", channelRule)
	}
	request("POST", "/alerts/rules/preview", channelBody, token, 200)
	request("GET", "/alerts/events?target_type=channel", "", token, 200)
	request("PATCH", path+"/enabled", `{"enabled":false}`, token, 200)
	if rule, _ := repo.FindRule(ruleID); rule.Enabled {
		t.Fatal("pause did not persist")
	}
	request("PATCH", path+"/enabled", `{}`, token, 400)
	request("POST", path+"/test", `{}`, token, 200)
	states, _ := repo.ListRuleStates(ruleID)
	events, _ := repo.ListEvents(storage.AlertEventFilter{})
	if len(states) != 0 || len(events) != 0 {
		t.Fatal("preview wrote runtime state")
	}
	now := time.Now().UTC()
	event := storage.AlertEvent{RuleID: ruleID, TargetType: "relay_account", RelayStationID: 7, TargetExternalID: "42", Severity: "critical", Metric: "account_spend", State: "firing", StartedAt: now, UpdatedAt: now}
	if err := repo.UpsertOpenEvent(&event); err != nil {
		t.Fatal(err)
	}
	if err := notifies.AppendLog(&storage.NotificationLog{ChannelID: channel.ID, AlertRuleID: ruleID, AlertEventID: event.ID, Event: storage.EventAlertFiring, Subject: "触发通知", Success: true}); err != nil {
		t.Fatal(err)
	}
	eventPath := fmt.Sprintf("/alerts/events/%d", event.ID)
	detail := request("GET", eventPath, "", token, 200)
	if !bytes.Contains(detail, []byte(`"notifications":[{`)) || !bytes.Contains(detail, []byte(`"rule":{`)) {
		t.Fatalf("missing event audit data: %s", detail)
	}
	request("DELETE", path, "", token, 400)
	request("GET", "/alerts/events?state=open&target_type=relay_account&severity=critical&station_id=7", "", token, 200)
	request("GET", "/alerts/events?offset=-1", "", token, 400)
	request("GET", "/alerts/events?from=invalid", "", token, 400)
	future := request("GET", "/alerts/events?from="+time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "", token, 200)
	if !bytes.Contains(future, []byte(`"data":[]`)) {
		t.Fatal("time filter included an earlier event")
	}
	request("GET", "/alerts/events?severity=invalid", "", token, 400)
	request("POST", eventPath+"/ack", `{}`, token, 200)
	request("POST", eventPath+"/recover", `{}`, token, 200)
	request("POST", eventPath+"/recover", `{}`, token, 200)
	events, _ = repo.ListEvents(storage.AlertEventFilter{State: "open"})
	if len(events) != 0 {
		t.Fatal("closed event still listed as open")
	}
	request("GET", "/alerts/settings", "", token, 200)
	request("PUT", "/alerts/settings", `{"enabled":true,"evaluation_interval_seconds":1,"rate_interval_seconds":60,"usage_interval_seconds":60,"balance_interval_seconds":300}`, token, 400)
	request("DELETE", path, "", token, 200)
	request("GET", path, "", token, 404)
}
