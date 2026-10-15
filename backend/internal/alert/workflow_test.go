package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/notify"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type scenarioCollector struct {
	db       *gorm.DB
	value    float64
	valid    bool
	observed time.Time
}

func (c *scenarioCollector) Collect(_ context.Context, rule storage.AlertRule, _ ConditionConfig, _ time.Time) ([]TargetObservation, error) {
	return []TargetObservation{{StationID: 7, TargetExternalID: "42", TargetName: "测试账号", Metric: MetricKind(rule.Metric), Observation: Observation{ObservedAt: c.observed, Value: c.value, Valid: c.valid, RequestCount: 10}}}, nil
}

type scenarioActions struct {
	fakeActions
	enabled      bool
	revision     int
	calls        int
	fail         bool
	readFailures int
}

func (a *scenarioActions) ReadAutomationState(context.Context, uint, string, string) (bool, string, error) {
	if a.readFailures > 0 {
		a.readFailures--
		return false, "", fmt.Errorf("read timeout")
	}
	return a.enabled, fmt.Sprint(a.revision), nil
}
func (a *scenarioActions) SetAutomationState(_ context.Context, _ uint, _, _ string, enabled bool) error {
	a.calls++
	if a.fail {
		return fmt.Errorf("remote timeout")
	}
	a.enabled = enabled
	a.revision++
	return nil
}

func workflowFixture(t *testing.T) (*Service, *storage.Alerts, *scenarioCollector, *scenarioActions) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alerts.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sql.Close() })
	if err := db.AutoMigrate(&storage.AlertRule{}, &storage.AlertRuleState{}, &storage.AlertEvent{}, &storage.AlertMetricSample{}, &storage.AlertActionLog{}, &storage.AlertRuleNotificationChannel{}, &storage.NotificationChannel{}, &storage.NotificationLog{}); err != nil {
		t.Fatal(err)
	}
	repo := storage.NewAlerts(db)
	collector := &scenarioCollector{db: db, valid: true, observed: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}
	actions := &scenarioActions{enabled: true, revision: 1}
	svc := NewService(repo, collector, actions, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, repo, collector, actions
}

func workflowRule(t *testing.T, svc *Service, recover bool) storage.AlertRule {
	t.Helper()
	rule := storage.AlertRule{Name: "消耗保护", TargetType: "relay_account", Metric: "account_spend", Enabled: true, ScopeJSON: `{"station_ids":[7]}`, ConditionJSON: `{"threshold":10,"recovery_threshold":5,"window_seconds":900,"trigger_consecutive":3,"recovery_consecutive":3}`, ActionJSON: `{"actions":[{"kind":"disable_account_scheduling","enabled":true}]}`, RecoveryJSON: fmt.Sprintf(`{"enabled":%t}`, recover), CooldownSeconds: 1800}
	if err := svc.CreateRule(&rule, nil); err != nil {
		t.Fatal(err)
	}
	return rule
}

func evaluateScenario(t *testing.T, svc *Service, collector *scenarioCollector, value float64, count int) {
	t.Helper()
	collector.value = value
	for i := 0; i < count; i++ {
		collector.observed = collector.observed.Add(time.Minute)
		if err := svc.EvaluateAll(context.Background(), collector.observed); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkflowTriggerRecoverAndReentry(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	workflowRule(t, svc, true)
	evaluateScenario(t, svc, collector, 20, 2)
	if actions.calls != 0 {
		t.Fatal("fired before three observations")
	}
	evaluateScenario(t, svc, collector, 20, 3)
	if actions.calls != 1 || actions.enabled {
		t.Fatal("disable must execute exactly once")
	}
	events, err := repo.ListEvents(storage.AlertEventFilter{})
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %+v %v", events, err)
	}
	evaluateScenario(t, svc, collector, 7, 3)
	if actions.enabled {
		t.Fatal("hysteresis band must not recover")
	}
	collector.valid = false
	evaluateScenario(t, svc, collector, 0, 3)
	if actions.enabled {
		t.Fatal("missing telemetry must not recover")
	}
	collector.valid = true
	evaluateScenario(t, svc, collector, 1, 3)
	if actions.calls != 2 || !actions.enabled {
		t.Fatal("expected safe automatic recovery")
	}
	logs, err := repo.ListActionLogs(events[0].ID)
	if err != nil || len(logs) != 2 || !logs[0].Success || !logs[1].Success {
		t.Fatalf("logs: %+v %v", logs, err)
	}
	evaluateScenario(t, svc, collector, 20, 3)
	events, _ = repo.ListEvents(storage.AlertEventFilter{})
	if len(events) != 2 || actions.calls != 3 {
		t.Fatal("new incident must have independent action")
	}
}

func TestWorkflowManualInterventionAndPreexistingDisable(t *testing.T) {
	for _, scenario := range []string{"manual", "preexisting", "no-recovery", "failed"} {
		t.Run(scenario, func(t *testing.T) {
			svc, repo, collector, actions := workflowFixture(t)
			workflowRule(t, svc, scenario != "no-recovery")
			if scenario == "preexisting" {
				actions.enabled = false
			}
			if scenario == "failed" {
				actions.fail = true
			}
			evaluateScenario(t, svc, collector, 20, 3)
			calls := actions.calls
			if scenario == "manual" {
				if err := repo.MarkManualIntervention("relay_account", 7, "42"); err != nil {
					t.Fatal(err)
				}
			}
			evaluateScenario(t, svc, collector, 1, 6)
			if actions.calls != calls {
				t.Fatalf("unsafe recovery/retry in %s: %d -> %d", scenario, calls, actions.calls)
			}
		})
	}
}

func TestDuplicateSamplesAndRuleCRUD(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	rule := workflowRule(t, svc, true)
	collector.value = 20
	for i := 0; i < 6; i++ {
		if err := svc.EvaluateAll(context.Background(), collector.observed); err != nil {
			t.Fatal(err)
		}
	}
	if actions.calls != 0 {
		t.Fatal("same sample counted multiple times")
	}
	rules, err := svc.ListRules(storage.AlertRuleFilter{})
	if err != nil || len(rules) != 1 {
		t.Fatalf("list: %+v %v", rules, err)
	}
	rule.Enabled = false
	if err := svc.UpdateRule(&rule, nil); err != nil {
		t.Fatal(err)
	}
	enabled := true
	rules, err = repo.ListRules(storage.AlertRuleFilter{Enabled: &enabled})
	if err != nil || len(rules) != 0 {
		t.Fatal("disabled flag not persisted")
	}
	if err := repo.UpdateRule(&storage.AlertRule{ID: 99999}, nil); err == nil {
		t.Fatal("update silently inserted missing rule")
	}
	var config ConditionConfig
	_ = json.Unmarshal([]byte(rule.ConditionJSON), &config)
	if config.TriggerConsecutive != 3 {
		t.Fatal("condition defaults lost")
	}
}

func TestCompetingRulesDelayRecovery(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	owner := workflowRule(t, svc, true)
	other := owner
	other.ID = 0
	other.Name = "另一条保护"
	other.CreatedAt = time.Time{}
	other.UpdatedAt = time.Time{}
	if err := svc.CreateRule(&other, nil); err != nil {
		t.Fatal(err)
	}
	evaluateScenario(t, svc, collector, 20, 3)
	if actions.calls != 1 {
		t.Fatalf("duplicate writes: %d", actions.calls)
	}
	evaluateScenario(t, svc, collector, 1, 6)
	if actions.calls != 2 || !actions.enabled {
		t.Fatal("owner did not wait for all holds to release")
	}
	states, _ := repo.ListRuleStates(owner.ID)
	if len(states) != 1 || (states[0].State != StateRecovered && states[0].State != StateNormal) {
		t.Fatalf("owner: %+v", states)
	}
}

func TestConcurrentEvaluationsExecuteActionOnce(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	workflowRule(t, svc, true)
	evaluateScenario(t, svc, collector, 20, 2)
	collector.observed = collector.observed.Add(time.Minute)
	var workers sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			errors <- svc.EvaluateAll(context.Background(), collector.observed)
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	events, err := repo.ListEvents(storage.AlertEventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || actions.calls != 1 || actions.enabled {
		t.Fatalf("concurrent evaluation produced %d events and %d actions", len(events), actions.calls)
	}
}

func TestRecoveryDeliveryFailureDoesNotBlockNewIncident(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	rule := workflowRule(t, svc, true)
	failDelivery := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failDelivery {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	cipher, _ := crypto.NewCipher("workflow-test-secret")
	secret, _ := cipher.Encrypt(fmt.Sprintf(`{"url":%q}`, server.URL))
	notifications := storage.NewNotifications(collector.db)
	ch := storage.NotificationChannel{Name: "恢复通知", Type: storage.NotifyWebhook, ConfigCipher: secret, Enabled: true}
	if err := notifications.CreateChannel(&ch); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateRule(&rule, []uint{ch.ID}); err != nil {
		t.Fatal(err)
	}
	svc.dispatcher = notify.NewDispatcher(notifications, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)), notify.Policy{SendMaxAttempts: 1})
	evaluateScenario(t, svc, collector, 20, 3)
	failDelivery = true
	evaluateScenario(t, svc, collector, 1, 2)
	collector.observed = collector.observed.Add(time.Minute)
	_ = svc.EvaluateAll(context.Background(), collector.observed)
	events, _ := repo.ListEvents(storage.AlertEventFilter{})
	if len(events) != 1 || events[0].RecoveredAt == nil || !actions.enabled {
		t.Fatal("notification failure must not retain an open incident after recovery")
	}
	failDelivery = false
	evaluateScenario(t, svc, collector, 20, 3)
	events, _ = repo.ListEvents(storage.AlertEventFilter{})
	if len(events) != 2 || actions.calls != 3 || actions.enabled {
		t.Fatal("next incident did not disable independently")
	}
	var delivered int64
	if err := collector.db.Model(&storage.NotificationLog{}).Where("event = ? AND success = ?", storage.EventAlertRecovered, true).Count(&delivered).Error; err != nil {
		t.Fatal(err)
	}
	if delivered != 1 {
		t.Fatalf("pending recovery deliveries = %d, want 1", delivered)
	}
}

func TestFailedRemoteReadStillRegistersDisableHold(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	rule := workflowRule(t, svc, true)
	actions.readFailures = 1
	evaluateScenario(t, svc, collector, 20, 3)
	states, _ := repo.ListRuleStates(rule.ID)
	var ownership storage.AlertAutomationOwnership
	if len(states) != 1 {
		t.Fatal("missing state")
	}
	if err := json.Unmarshal([]byte(states[0].AutomaticActionJSON), &ownership); err != nil {
		t.Fatal(err)
	}
	if !ownership.Hold || ownership.Owned {
		t.Fatal("failed read must still block competing automatic enable actions")
	}
}

func TestChangingPendingConditionResetsOldEvidence(t *testing.T) {
	svc, repo, collector, actions := workflowFixture(t)
	rule := workflowRule(t, svc, true)
	rule.ConditionJSON = `{"threshold":10,"recovery_threshold":5,"window_seconds":900,"trigger_consecutive":3,"recovery_consecutive":3,"hit_window":5}`
	if err := svc.UpdateRule(&rule, nil); err != nil {
		t.Fatal(err)
	}
	evaluateScenario(t, svc, collector, 20, 2)
	rule.ConditionJSON = `{"threshold":10,"recovery_threshold":5,"window_seconds":900,"trigger_consecutive":2,"recovery_consecutive":3,"hit_window":5}`
	if err := svc.UpdateRule(&rule, nil); err != nil {
		t.Fatal(err)
	}
	evaluateScenario(t, svc, collector, 1, 1)
	states, _ := repo.ListRuleStates(rule.ID)
	if len(states) != 1 || states[0].State != StateNormal || actions.calls != 0 {
		t.Fatal("old rule evidence leaked into edited condition")
	}
}
