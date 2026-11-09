package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/notify"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestUserInactiveThresholdBoundaries(t *testing.T) {
	for _, tc := range []struct {
		days     float64
		violated bool
	}{{29.999, false}, {30, true}, {45, true}, {0, false}} {
		result := EvaluateCondition(RuleSpec{Metric: MetricKind("user_inactive"), Threshold: 30}, Observation{Value: tc.days, Valid: true}, nil)
		if result.Unknown || result.Violated != tc.violated || result.Actual != tc.days {
			t.Fatalf("%+v: %+v", tc, result)
		}
	}
}

func TestUserInactiveRuleValidationAndGenericActions(t *testing.T) {
	for _, action := range []string{"", "disable_user", "enable_user"} {
		rule := storage.AlertRule{Name: "inactive", TargetType: "relay_user", Metric: "user_inactive", ConditionJSON: `{}`, ScopeJSON: `{"station_ids":[7]}`, CooldownSeconds: 1800}
		if action != "" {
			rule.ActionJSON = fmt.Sprintf(`{"actions":[{"kind":%q,"enabled":true}]}`, action)
		}
		if err := validateRule(&rule); err != nil {
			t.Fatal(err)
		}
		var config ConditionConfig
		if err := json.Unmarshal([]byte(rule.ConditionJSON), &config); err != nil {
			t.Fatal(err)
		}
		if config.Threshold != 30 || config.MinRequests != 0 {
			t.Fatalf("%+v", config)
		}
	}
	for _, days := range []float64{0, -1, 0.5, 3651} {
		rule := storage.AlertRule{Name: "inactive", TargetType: "relay_user", Metric: "user_inactive", ConditionJSON: fmt.Sprintf(`{"threshold":%v}`, days), CooldownSeconds: 1800}
		if err := validateRule(&rule); err == nil {
			t.Fatalf("threshold = %v", days)
		}
	}
	for _, days := range []float64{1, 30, 3650} {
		rule := storage.AlertRule{Name: "inactive", TargetType: "relay_user", Metric: "user_inactive", ConditionJSON: fmt.Sprintf(`{"threshold":%v,"min_requests":10}`, days), CooldownSeconds: 1800}
		if err := validateRule(&rule); err != nil {
			t.Fatal(err)
		}
		var config ConditionConfig
		_ = json.Unmarshal([]byte(rule.ConditionJSON), &config)
		if config.Threshold != days || config.MinRequests != 0 {
			t.Fatalf("%+v", config)
		}
	}
	wrong := storage.AlertRule{Name: "inactive", TargetType: "relay_account", Metric: "user_inactive", ConditionJSON: `{"threshold":30}`, CooldownSeconds: 1800}
	if err := validateRule(&wrong); err == nil {
		t.Fatal("target")
	}
	if got := alertLabel("user_inactive"); got != "用户长期未使用" {
		t.Fatal(got)
	}
}

func TestUserInactivityObservation(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	old, recent, future := now.Add(-30*24*time.Hour), now.Add(-time.Hour), now.Add(time.Hour)
	for _, tc := range []struct {
		name      string
		last      *time.Time
		created   time.Time
		observed  time.Time
		supported bool
		valid     bool
		days      float64
	}{
		{"boundary", &old, old.Add(-24 * time.Hour), now, true, true, 30},
		{"recent", &recent, old, now, true, true, 1.0 / 24},
		{"never", nil, old, now, true, true, 30},
		{"new", nil, recent, now, true, true, 1.0 / 24},
		{"unsupported", nil, old, now, false, false, 0},
		{"missing", nil, time.Time{}, now, true, false, 0},
		{"stale", &old, old, now.Add(-4 * time.Minute), true, false, 0},
		{"future-usage", &future, old, now, true, false, 0},
		{"future-created", nil, future, now, true, false, 0},
		{"missing-snapshot", &old, old, time.Time{}, true, false, 0},
		{"future-snapshot", &old, old, future, true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := userInactivityObservation(storage.RelayUserSnapshot{LastUsedAt: tc.last, CreatedAt: tc.created, ObservedAt: tc.observed, LastUsedAtSupported: tc.supported}, now)
			if got.Valid != tc.valid || (tc.valid && got.Value != tc.days) || (!tc.valid && got.Reason == "") {
				t.Fatalf("%+v", got)
			}
		})
	}
}

type liveUserSnapshots struct{}

func (liveUserSnapshots) AlertUserSnapshots(context.Context, uint) ([]storage.RelayUserSnapshot, error) {
	observed := time.Now().UTC()
	return []storage.RelayUserSnapshot{{ID: 42, Status: "active", CreatedAt: observed.Add(-31 * 24 * time.Hour), ObservedAt: observed, LastUsedAtSupported: true}}, nil
}

func TestCollectUserInactiveWithoutUsageLogs(t *testing.T) {
	_, repo, fixture, _ := workflowFixture(t)
	db := fixture.db
	if err := db.AutoMigrate(&storage.RelayStation{}); err != nil {
		t.Fatal(err)
	}
	stations := storage.NewRelayStations(db)
	station := storage.RelayStation{Name: "Sub2API", Type: "sub2api", BaseURL: "https://example.invalid", APIKeyCipher: "cipher"}
	if err := stations.Create(&station); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshots := fixedUserSnapshots{
		{ID: 42, Username: "inactive-user", Status: "active", CreatedAt: now.Add(-31 * 24 * time.Hour), ObservedAt: now, LastUsedAtSupported: true},
		{ID: 43, Username: "disabled-user", Status: "disabled", CreatedAt: now.Add(-35 * 24 * time.Hour), ObservedAt: now, LastUsedAtSupported: true},
		{ID: 44, Username: "new-user", Status: "active", CreatedAt: now.Add(-24 * time.Hour), ObservedAt: now, LastUsedAtSupported: true},
	}
	scope, _ := json.Marshal(ScopeConfig{StationIDs: []uint{station.ID}, Filters: []ScopeFilter{{Field: "status", Operator: "eq", Value: "active"}}})
	rule := storage.AlertRule{ID: 5, TargetType: "relay_user", Metric: "user_inactive", ScopeJSON: string(scope)}
	if err := repo.SaveState(&storage.AlertRuleState{RuleID: rule.ID, TargetType: rule.TargetType, RelayStationID: station.ID, TargetExternalID: "43", State: storage.AlertStateFiring}); err != nil {
		t.Fatal(err)
	}
	collector := NewCollector(repo, stations, nil, CollectorSources{Users: snapshots})
	targets, err := collector.Collect(context.Background(), rule, ConditionConfig{Threshold: 30, WindowSeconds: 900}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 {
		t.Fatalf("%+v", targets)
	}
	for _, target := range targets {
		result := EvaluateCondition(RuleSpec{Metric: MetricKind("user_inactive"), Threshold: 30}, target.Observation, nil)
		if result.Unknown || result.Violated != (target.TargetExternalID != "44") || target.TargetName == target.TargetExternalID {
			t.Fatalf("%+v: %+v", target, result)
		}
	}
	collector.users = liveUserSnapshots{}
	targets, err = collector.Collect(context.Background(), rule, ConditionConfig{Threshold: 30}, now)
	if err != nil || len(targets) != 1 || !targets[0].Observation.Valid || targets[0].Observation.Value != 31 {
		t.Fatalf("post-fetch snapshot = %+v, %v", targets, err)
	}
	collector = NewCollector(repo, stations, nil)
	if _, err := collector.Collect(context.Background(), rule, ConditionConfig{Threshold: 30}, now); err == nil {
		t.Fatal("adapter")
	}
}

func TestUserInactiveUsesGenericDisableAndRecovery(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	rule := storage.AlertRule{Name: "inactive", TargetType: "relay_user", Metric: "user_inactive", Enabled: true, ScopeJSON: `{"station_ids":[7]}`, ConditionJSON: `{"threshold":30,"trigger_consecutive":1,"recovery_consecutive":1}`, ActionJSON: `{"actions":[{"kind":"disable_user","enabled":true}]}`, RecoveryJSON: `{"enabled":false}`, CooldownSeconds: 1800}
	if err := svc.CreateRule(&rule, nil); err != nil {
		t.Fatal(err)
	}
	evaluateScenario(t, svc, collector, 30, 1)
	if actions.calls != 1 || actions.enabled {
		t.Fatalf("%+v", actions)
	}
	events, err := repo.ListEvents(storage.AlertEventFilter{})
	if err != nil || len(events) != 1 {
		t.Fatalf("%+v %v", events, err)
	}
	logs, err := repo.ListActionLogs(events[0].ID)
	if err != nil || len(logs) != 1 || logs[0].Action != "disable_user" || !logs[0].Success {
		t.Fatalf("%+v %v", logs, err)
	}
	evaluateScenario(t, svc, collector, 0, 1)
	if actions.calls != 1 || actions.enabled {
		t.Fatalf("%+v", actions)
	}
	events, err = repo.ListEvents(storage.AlertEventFilter{})
	if err != nil || len(events) != 1 || events[0].RecoveredAt == nil {
		t.Fatalf("%+v %v", events, err)
	}
}

func TestUserInactiveUnknownNeverRunsGenericAction(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	rule := storage.AlertRule{Name: "inactive", TargetType: "relay_user", Metric: "user_inactive", Enabled: true, ConditionJSON: `{"threshold":30,"trigger_consecutive":1}`, ActionJSON: `{"actions":[{"kind":"disable_user","enabled":true}]}`, CooldownSeconds: 1800}
	if err := svc.CreateRule(&rule, nil); err != nil {
		t.Fatal(err)
	}
	collector.valid = false
	evaluateScenario(t, svc, collector, 90, 3)
	events, err := repo.ListEvents(storage.AlertEventFilter{})
	if err != nil || len(events) != 0 || actions.calls != 0 {
		t.Fatalf("%+v %+v %v", events, actions, err)
	}
	states, err := repo.ListRuleStates(rule.ID)
	if err != nil || len(states) != 1 || states[0].State != StateUnknown {
		t.Fatalf("%+v %v", states, err)
	}
}

func TestUserInactiveNotificationUsesDays(t *testing.T) {
	svc, _, collector, _ := workflowFixture(t)
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		body = payload.Body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	cipher, err := crypto.NewCipher("inactive-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := cipher.Encrypt(fmt.Sprintf(`{"url":%q}`, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	notifications := storage.NewNotifications(collector.db)
	channel := storage.NotificationChannel{Name: "inactive", Type: storage.NotifyWebhook, ConfigCipher: secret, Enabled: true}
	if err := notifications.CreateChannel(&channel); err != nil {
		t.Fatal(err)
	}
	svc.dispatcher = notify.NewDispatcher(notifications, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)), notify.Policy{SendMaxAttempts: 1})
	rule := storage.AlertRule{Name: "inactive", TargetType: "relay_user", Metric: "user_inactive", Enabled: true, ConditionJSON: `{"threshold":30,"trigger_consecutive":1}`, CooldownSeconds: 1800}
	if err := svc.CreateRule(&rule, []uint{channel.ID}); err != nil {
		t.Fatal(err)
	}
	evaluateScenario(t, svc, collector, 30, 1)
	for _, text := range []string{"用户长期未使用", "30.0000 天", "按最后使用时间判定"} {
		if !strings.Contains(body, text) {
			t.Fatalf("%q: %s", text, body)
		}
	}
	if strings.Contains(body, "900 秒") {
		t.Fatal(body)
	}
}
