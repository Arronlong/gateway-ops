package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/guliansheng/gateway-ops/internal/crypto"
	"github.com/guliansheng/gateway-ops/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func pelicanExecutionDB(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(storage.NewRelayStations(db), nil)
	service.settings = storage.NewSettings(db)
	return service, mock
}

func expectGlobalPelicanSettings(mock sqlmock.Sqlmock, model string) {
	expectGlobalPelicanWindow(mock, model, "00:00", "00:00")
}

func TestPelicanManualRunPlansUsesOneValidatedOverride(t *testing.T) {
	configured := []storage.PelicanTestPlan{
		{Model: "model-a,model-b", Prompt: "saved prompt", ReasoningEffort: "medium"},
		{Model: "model-c", Prompt: "another prompt", ReasoningEffort: "low"},
	}
	plans, err := pelicanManualRunPlans(configured, []storage.PelicanTestPlan{{Model: "model-b", Prompt: "one-off prompt", ReasoningEffort: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0] != (storage.PelicanTestPlan{Model: "model-b", Prompt: "one-off prompt", ReasoningEffort: "high"}) {
		t.Fatalf("unexpected one-off plan: %#v", plans)
	}
	defaults, err := pelicanManualRunPlans(configured, nil)
	if err != nil || len(defaults) != 2 || defaults[0] != configured[0] || defaults[1] != configured[1] {
		t.Fatalf("default run should keep configured plans: plans=%#v err=%v", defaults, err)
	}
}

func TestPelicanManualRunPlansRejectsInvalidOverrides(t *testing.T) {
	configured := []storage.PelicanTestPlan{{Model: "model-a,model-b", Prompt: "saved prompt", ReasoningEffort: "medium"}}
	for _, plan := range []storage.PelicanTestPlan{
		{Model: "unknown", Prompt: "prompt", ReasoningEffort: "medium"},
		{Model: "model-a", Prompt: " ", ReasoningEffort: "medium"},
		{Model: "model-a", Prompt: "prompt", ReasoningEffort: "invalid"},
	} {
		if _, err := pelicanManualRunPlans(configured, []storage.PelicanTestPlan{plan}); err == nil {
			t.Errorf("expected invalid override to be rejected: %#v", plan)
		}
	}
}

func expectGlobalPelicanWindow(mock sqlmock.Sqlmock, model, start, end string, tasksEnabled ...bool) {
	for _, field := range [][2]string{{storage.SettingPelicanModel, model}, {storage.SettingPelicanPrompt, "global drawing"}, {storage.SettingPelicanReasoningEffort, "high"}, {storage.SettingPelicanCronExpression, "0 * * * *"}, {storage.SettingPelicanActiveTimezone, "Asia/Shanghai"}, {storage.SettingPelicanActiveStart, start}, {storage.SettingPelicanActiveEnd, end}} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(field[0], 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(field[0], field[1]))
	}
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanActiveWeekdays, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(storage.SettingPelicanActiveWeekdays, "[0,1,2,3,4,5,6]"))
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanParallelCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(storage.SettingPelicanParallelCount, "1"))
	encoded, _ := json.Marshal([]storage.PelicanTestPlan{{Model: model, Prompt: "global drawing", ReasoningEffort: "high"}})
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanPlans, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(storage.SettingPelicanPlans, string(encoded)))
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(storage.SettingPelicanHistoryCount, "30"))
	// 隐藏页面不应阻止任务执行，现有执行测试一并覆盖这个组合。
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanPageEnabled, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(storage.SettingPelicanPageEnabled, "false"))
	enabled := "true"
	if len(tasksEnabled) > 0 && !tasksEnabled[0] {
		enabled = "false"
	}
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanTasksEnabled, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(storage.SettingPelicanTasksEnabled, enabled))
}

func TestPelicanDisabledTasksRejectManualBeforeClaim(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	mock.ExpectQuery(`SELECT .*relay_stations`).WithArgs(7, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "type"}).AddRow(7, "sub2api"))
	expectGlobalPelicanWindow(mock, "global-model", "00:00", "00:00", false)
	if _, err := service.StartPelican(context.Background(), 7, 20); err == nil || !strings.Contains(err.Error(), "任务执行已关闭") {
		t.Fatalf("disabled manual execution must fail before claiming or reading credentials: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanDisabledTasksSkipScheduledAccounts(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	expectGlobalPelicanWindow(mock, "global-model", "00:00", "00:00", false)
	if err := service.runDuePelicansAt(context.Background(), time.Now()); err != nil {
		t.Fatalf("disabled scheduler must exit without querying or claiming accounts: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavePelicanAccountUsesGlobalSettingsAndOnlyAccountSwitch(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "type"}).AddRow(7, "sub2api"))
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_configs"`).WithArgs(uint(7), int64(20), true, "global-model", "global drawing", "high", 1, "0 * * * *", sqlmock.AnyArg(), nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id", "enabled"}).AddRow(1, true))
	config, err := service.SavePelicanConfig(7, 20, storage.PelicanConfig{Enabled: true, Model: "must-not-override-global", Prompt: "ignored", ParallelCount: 8})
	if err != nil || !config.Enabled {
		t.Fatalf("config=%#v err=%v", config, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManualPelicanUsesGlobalSettingsEvenWhenScheduleDisabledAndOutsideWindow(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "type"}).AddRow(7, "sub2api"))
	beijing := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	expectGlobalPelicanWindow(mock, "global-model", beijing.Add(time.Hour).Format("15:04"), beijing.Add(2*time.Hour).Format("15:04"))
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "account_external_id", "enabled", "model", "prompt", "parallel_count"}).AddRow(1, 7, 20, false, "legacy-model", "legacy", 8))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(20, "test account"))
	expectPelicanPlaceholder(mock, "global-model", "global drawing", "high", "manual", 1)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec(`UPDATE "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .*app_settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	config, err := service.StartPelican(context.Background(), 7, 20)
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.Model != "global-model" || config.Prompt != "global drawing" || config.ParallelCount != 1 || config.LeaseUntil == nil {
		t.Fatalf("manual execution must keep schedule disabled and use global settings: %#v", config)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err := mock.ExpectationsWereMet()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScheduledPelicanSkipsGenerationUntilGlobalModelConfigured(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	expectGlobalPelicanSettings(mock, "")
	if err := service.RunDuePelicans(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestScheduledPelicanSkipsOutsideAllowedWindow(t *testing.T) {
	for _, hour := range []int{7, 22, 23} {
		t.Run(fmt.Sprint(hour), func(t *testing.T) {
			service, mock := pelicanExecutionDB(t)
			expectGlobalPelicanWindow(mock, "global-model", "08:00", "22:00")
			now := time.Date(2026, 10, 22, hour, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
			if err := service.runDuePelicansAt(context.Background(), now); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestScheduledPelicanReschedulesNighttimeDueWithoutGenerating(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery(`SELECT c.* FROM relay_group_pelican_configs c`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	expectGlobalPelicanWindow(mock, "global-model", "08:00", "22:00")
	now := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT c\.\* FROM relay_pelican_configs c`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "account_external_id", "enabled", "next_run_at"}).AddRow(1, 7, 20, true, now.Add(-time.Hour)))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.runDuePelicansAt(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManualPelicanCreatesDisabledAccountConfigOnFirstRun(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	stationRows := func() *sqlmock.Rows { return sqlmock.NewRows([]string{"id", "type"}).AddRow(7, "sub2api") }
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(stationRows())
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(stationRows())
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(stationRows())
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_configs"`).WithArgs(uint(7), int64(20), false, "global-model", "global drawing", "high", 1, "0 * * * *", sqlmock.AnyArg(), nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_pelican_configs`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "account_external_id", "enabled"}).AddRow(1, 7, 20, false))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(20))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(20, "test account"))
	expectPelicanPlaceholder(mock, "global-model", "global drawing", "high", "manual", 1)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec(`UPDATE "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .*app_settings`).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	config, err := service.StartPelican(context.Background(), 7, 20)
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled || config.Model != "global-model" || config.LeaseUntil == nil {
		t.Fatalf("first manual run must not enable the account schedule: %#v", config)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err := mock.ExpectationsWereMet()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestScheduledPelicanLoadsGlobalParametersInsteadOfLegacyAccountParameters(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery(`SELECT c.* FROM relay_group_pelican_configs c`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	now := time.Now().UTC().Truncate(time.Microsecond)
	expectGlobalPelicanSettings(mock, "global-model")
	mock.ExpectQuery(`SELECT c\.\* FROM relay_pelican_configs c`).WillReturnRows(sqlmock.NewRows([]string{"id", "relay_station_id", "account_external_id", "enabled", "model", "prompt", "parallel_count", "cron_expression", "next_run_at"}).AddRow(1, 7, 20, true, "legacy-model", "legacy drawing", 8, "invalid-legacy-cron", now.Add(-time.Minute)))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WithArgs(uint(7), int64(20), "账号 #20", "global-model", "global drawing", "high", "scheduled", "failed", "", "record not found", int64(0), sqlmock.AnyArg(), sqlmock.AnyArg(), "", nil).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.RunDuePelicans(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanMissingAccountCreatesFailureHistory(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	config := storage.PelicanConfig{ID: 1, RelayStationID: 7, AccountExternalID: 20, Model: "test-model", Prompt: "draw", ReasoningEffort: "medium", ParallelCount: 1, NextRunAt: time.Now().Add(-time.Minute)}
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WithArgs(uint(7), int64(20), "账号 #20", "test-model", "draw", "medium", "scheduled", "failed", "", "record not found", int64(0), sqlmock.AnyArg(), sqlmock.AnyArg(), "", nil).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at.*WHERE id = \$5 AND lease_until = \$6`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	service.runPelicanBatch(context.Background(), config, time.Now().Add(time.Minute), time.Now().Add(time.Hour), "scheduled")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanFailedPersistenceKeepsLeaseAndDueTimeForRecovery(t *testing.T) {
	service, mock := pelicanExecutionDB(t)
	config := storage.PelicanConfig{ID: 1, RelayStationID: 7, AccountExternalID: 20, ParallelCount: 1}
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WillReturnError(errors.New("database temporarily unavailable"))
	mock.ExpectRollback()
	service.runPelicanBatch(context.Background(), config, time.Now().Add(time.Minute), time.Now().Add(time.Hour), "scheduled")
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanNoTextModelSavesFailureAndReleasesLease(t *testing.T) {
	for _, trigger := range []string{"manual", "scheduled"} {
		t.Run(trigger, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/20/models" {
					t.Errorf("unexpected generation: %s %s", r.Method, r.URL.Path)
				}
				w.Write([]byte(`{"code":0,"data":["text-embedding-3-large"]}`))
			}))
			defer server.Close()
			service, mock := pelicanExecutionDB(t)
			config := storage.PelicanConfig{ID: 1, RelayStationID: 7, AccountExternalID: 20, Model: "missing-model", Prompt: "draw", ReasoningEffort: "medium", ParallelCount: 8, NextRunAt: time.Now().Add(-time.Minute)}
			mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "base_url"}).AddRow(7, server.URL))
			mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(20, "test account"))
			errorText := "\u8be5\u8d26\u53f7\u4e0d\u652f\u6301\u7cfb\u7edf\u914d\u7f6e\u7684\u6d4b\u667a\u6a21\u578b \"missing-model\"\uff1b\u53ef\u7528\u6a21\u578b: text-embedding-3-large"
			for i := 0; i < 1; i++ {
				expectPelicanPlaceholder(mock, "missing-model", "draw", "medium", trigger, uint(i+1))
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
				mock.ExpectExec(`UPDATE "relay_pelican_results" SET .* WHERE id = .*relay_station_id = .*account_external_id =`).WithArgs(uint(7), int64(20), "test account", "missing-model", "draw", "medium", trigger, "failed", "", errorText, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "", nil, uint(i+1), uint(7), int64(20)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
				mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectCommit()
			}
			mock.ExpectBegin()
			if trigger == "scheduled" {
				mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at`).WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			service.runPelicanBatch(context.Background(), config, time.Now().Add(time.Minute), time.Now().Add(time.Hour), trigger)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("preflight requests=%d, expected once for the configured plan", calls)
			}
		})
	}
}

func TestPelicanPriorityFallbackPersistsActualModelWithoutChangingSettings(t *testing.T) {
	for _, trigger := range []string{"manual", "scheduled"} {
		t.Run(trigger, func(t *testing.T) {
			generationCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/admin/accounts/20/models":
					w.Write([]byte(`{"code":0,"data":["glm-5.3","deepseek-flash","kimi-k3"]}`))
				case "/api/v1/admin/accounts/data":
					w.Write([]byte(`{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://model.invalid","api_key":"upstream-secret","model_mapping":{"deepseek-flash":"actual-deepseek-flash"}},"extra":{"openai_responses_supported":false}}]}}`))
				case "/v1/chat/completions":
					generationCalls++
					w.Write([]byte(`{"choices":[{"message":{"content":"<svg></svg>"},"finish_reason":"stop"}]}`))
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			service, mock := pelicanExecutionDB(t)
			service.client.Transport = pelicanRewriteTransport{base: server.Client().Transport, target: server.URL}
			config := storage.PelicanConfig{ID: 1, RelayStationID: 7, AccountExternalID: 20, Model: "gpt-6.1-sol\ndeepseek-flash", Prompt: "draw", ReasoningEffort: "medium", ParallelCount: 1, NextRunAt: time.Now().Add(-time.Minute)}
			mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "base_url"}).AddRow(7, server.URL))
			mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(20, "test account"))
			expectPelicanPlaceholder(mock, "gpt-6.1-sol\ndeepseek-flash", "draw", "medium", trigger, uint(1))
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
			mock.ExpectExec(`UPDATE "relay_pelican_results" SET .* WHERE id = .*relay_station_id = .*account_external_id =`).WithArgs(uint(7), int64(20), "test account", "actual-deepseek-flash", "draw", "medium", trigger, "success", "<svg></svg>", "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "", nil, uint(1), uint(7), int64(20)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
			mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectCommit()
			mock.ExpectBegin()
			if trigger == "scheduled" {
				mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at`).WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
			service.runPelicanBatch(context.Background(), config, time.Now().Add(time.Minute), time.Now().Add(time.Hour), trigger)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
			if generationCalls != 1 || config.Model != "gpt-6.1-sol\ndeepseek-flash" {
				t.Fatalf("calls=%d configured_model=%q", generationCalls, config.Model)
			}
		})
	}
}

func TestPelicanRunsEachIndependentPlanConcurrently(t *testing.T) {
	for _, trigger := range []string{"manual", "scheduled"} {
		for _, sameModel := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/same-model-%t", trigger, sameModel), func(t *testing.T) { testPelicanRunsEachIndependentPlanConcurrently(t, trigger, sameModel) })
		}
	}
}

func testPelicanRunsEachIndependentPlanConcurrently(t *testing.T, trigger string, sameModel bool) {
	var active, peak atomic.Int32
	var keyLookups atomic.Int32
	var ready atomic.Int32
	started := make(chan struct{})
	var startedOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/accounts/20/models":
			w.Write([]byte(`{"code":0,"data":["model-a","model-b"]}`))
		case "/api/v1/admin/accounts/data":
			w.Write([]byte(`{"code":0,"data":{"accounts":[{"platform":"openai","type":"apikey","credentials":{"base_url":"http://model.invalid","api_key":"upstream-secret"},"extra":{"openai_responses_supported":false}}]}}`))
		case "/api/v1/keys":
			keyLookups.Add(1)
			w.Write([]byte(`{"code":0,"data":{"items":[{"key":"upstream-secret","group_id":2,"group":{"id":2,"name":"生成 Key 分组","rate_multiplier":3}}],"pages":1}}`))
		case "/v1/sub2api/billing":
			w.Write([]byte(`{"object":"sub2api.key_billing","resolved_rate_multiplier":0.25}`))
		case "/v1/chat/completions":
			current := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); current > old && !peak.CompareAndSwap(old, current); old = peak.Load() {
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
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if len(request.Messages) != 1 || !strings.HasSuffix(request.Messages[0].Content, pelicanContract) {
				t.Error("each plan must append the standalone HTML requirement")
			}
			if ready.Add(1) == 2 {
				startedOnce.Do(func() { close(started) })
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Errorf("plans did not run concurrently")
			}
			content := "<svg>" + request.Model + ":" + request.Effort + ":" + request.Messages[0].Content + "</svg>"
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	service, mock := pelicanExecutionDB(t)
	service.client.Transport = pelicanRewriteTransport{base: server.Client().Transport, target: server.URL}
	mock.MatchExpectationsInOrder(false)
	service.channels = storage.NewChannels(nil)
	service.cipher, _ = crypto.NewCipher("pelican-metadata-test-secret")
	sessionCipher, _ := service.cipher.Encrypt("login-session")
	mock.ExpectQuery(`SELECT channels.site_url, sessions.access_token_cipher`).WithArgs(storage.AdditionalAccountSessionOffset, uint(7), int64(20), "sub2api", "newapi").WillReturnRows(sqlmock.NewRows([]string{"site_url", "access_token_cipher"}).AddRow("http://model.invalid", sessionCipher))
	config := storage.PelicanConfig{ID: 1, RelayStationID: 7, AccountExternalID: 20, ParallelCount: 1}
	mock.ExpectQuery(`SELECT .*relay_stations`).WillReturnRows(sqlmock.NewRows([]string{"id", "base_url"}).AddRow(7, server.URL))
	mock.ExpectQuery(`SELECT .*relay_accounts`).WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(20, "test account"))
	secondModel := "model-b"
	if sameModel {
		secondModel = "model-a"
	}
	plans := []storage.PelicanTestPlan{{Model: "model-a", Prompt: "draw A", ReasoningEffort: "low"}, {Model: secondModel, Prompt: "draw B", ReasoningEffort: "high"}, {Model: "missing-model", Prompt: "draw unavailable", ReasoningEffort: "medium"}}
	for i, plan := range plans {
		status := "success"
		groupName := "生成 Key 分组"
		var groupRate any = float64(0.25)
		var errorValue sqlmock.Argument = sqlmock.AnyArg()
		if plan.Model == "missing-model" {
			status = "failed"
			groupName, groupRate = "", nil
		}
		expectPelicanPlaceholder(mock, plan.Model, plan.Prompt, plan.ReasoningEffort, trigger, uint(i+1))
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
		mock.ExpectExec(`UPDATE "relay_pelican_results" SET .* WHERE id = .*relay_station_id = .*account_external_id =`).WithArgs(uint(7), int64(20), "test account", plan.Model, plan.Prompt, plan.ReasoningEffort, trigger, status, sqlmock.AnyArg(), errorValue, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), groupName, groupRate, uint(i+1), uint(7), int64(20)).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(storage.SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
		mock.ExpectExec(`DELETE FROM "relay_pelican_results"`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
	}
	mock.ExpectBegin()
	if trigger == "scheduled" {
		mock.ExpectExec(`UPDATE "relay_pelican_configs".*CASE WHEN next_run_at`).WillReturnResult(sqlmock.NewResult(0, 1))
	} else {
		mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .*lease_until`).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	service.runPelicanBatch(context.Background(), config, time.Now().Add(time.Minute), time.Now().Add(time.Hour), trigger, plans)
	if peak.Load() != 2 {
		t.Fatalf("peak concurrent generations=%d, want 2", peak.Load())
	}
	if keyLookups.Load() != 1 {
		t.Fatalf("group metadata must be shared across plans: %d", keyLookups.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectPelicanPlaceholder(mock sqlmock.Sqlmock, model, prompt, effort, trigger string, id uint) {
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*relay_stations.*FOR UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectQuery(`INSERT INTO "relay_pelican_results"`).WithArgs(uint(7), int64(20), "test account", model, prompt, effort, trigger, "running", "", "", int64(0), sqlmock.AnyArg(), sqlmock.AnyArg(), "", nil).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
	mock.ExpectCommit()
}
