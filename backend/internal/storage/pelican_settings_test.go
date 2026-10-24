package storage

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGlobalPelicanScheduleUsesBeijingTime(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 59, 0, 0, time.UTC)
	next, err := NextPelicanRun("0 9 * * *", now)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Fatalf("next=%v, want %v", next, want)
	}
	for _, expression := range []string{"* * * * * *", "invalid", "0 0 31 2 *"} {
		if _, err := NextPelicanRun(expression, now); err == nil {
			t.Errorf("accepted invalid schedule %q", expression)
		}
	}
}

func TestGlobalPelicanSettingsRejectInvalidOptions(t *testing.T) {
	for _, update := range []func(*PelicanSettings){
		func(s *PelicanSettings) { s.Model = strings.Repeat("a", 257) },
		func(s *PelicanSettings) { s.Prompt = strings.Repeat("a", 32001) },
		func(s *PelicanSettings) { s.ParallelCount = -1 },
		func(s *PelicanSettings) { s.ReasoningEffort = "bogus" },
	} {
		settings := PelicanSettings{Prompt: DefaultPelicanPrompt, ReasoningEffort: "medium", ParallelCount: 1, CronExpression: "*/30 * * * *", ActiveStart: "08:00", ActiveEnd: "22:00"}
		update(&settings)
		settings.Plans = []PelicanTestPlan{{Model: settings.Model, Prompt: settings.Prompt, ReasoningEffort: settings.ReasoningEffort}}
		if err := ValidatePelicanSettings(settings); err == nil {
			t.Fatalf("accepted invalid settings %#v", settings)
		}
	}
}

func TestPelicanPlansValidateIndependentlyAndKeepLegacyFallback(t *testing.T) {
	base := defaultUISettings().PelicanSettings()
	base.Plans = []PelicanTestPlan{
		{Model: "first-model, fallback-model", Prompt: "draw one", ReasoningEffort: "medium"},
		{Model: "first-model", Prompt: "draw two", ReasoningEffort: "high"},
	}
	if err := ValidatePelicanSettings(base); err != nil {
		t.Fatal(err)
	}
	base.Plans[1].Prompt = " "
	if err := ValidatePelicanSettings(base); err == nil {
		t.Fatal("expected per-plan prompt validation")
	}
	base = defaultUISettings().PelicanSettings()
	base.Model, base.Prompt, base.ReasoningEffort = "legacy-model", "legacy prompt", "high"
	if got := normalizedPelicanPlans(nil, base.Model, base.Prompt, base.ReasoningEffort); len(got) != 1 || got[0].Model != "legacy-model" || got[0].ReasoningEffort != "high" {
		t.Fatalf("legacy settings were not mapped to one plan: %#v", got)
	}
}

func TestPelicanExplicitEmptyPlansNeverRestoreStaleLegacyModel(t *testing.T) {
	got := normalizedPelicanPlans([]PelicanTestPlan{}, "stale-model", "stale prompt", "high")
	if len(got) != 1 || got[0].Model != "" || got[0].Prompt != DefaultPelicanPrompt || got[0].ReasoningEffort != "medium" || len(ConfiguredPelicanPlans(got)) != 0 {
		t.Fatalf("explicit empty plans must become an unconfigured default, got %#v", got)
	}
}

func TestPelicanPlansRejectEmptyAndExcessConfigurations(t *testing.T) {
	base := defaultUISettings().PelicanSettings()
	base.Plans = make([]PelicanTestPlan, 9)
	for index := range base.Plans {
		base.Plans[index] = PelicanTestPlan{Model: "model", Prompt: "draw", ReasoningEffort: "medium"}
	}
	if err := ValidatePelicanSettings(base); err == nil {
		t.Fatal("expected plan count validation")
	}
	base.Plans = []PelicanTestPlan{{Model: "model", Prompt: "draw", ReasoningEffort: "medium"}, {Prompt: "draw", ReasoningEffort: "high"}}
	if err := ValidatePelicanSettings(base); err == nil {
		t.Fatal("expected each configured plan to have a model")
	}
}

func expectPelicanSettingWrite(mock sqlmock.Sqlmock, key, value string) {
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectExec(`INSERT INTO "app_settings"`).WithArgs(key, value, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestPelicanSettingsReadGlobalValues(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.MatchExpectationsInOrder(false)
	values := map[string]string{
		SettingPelicanModel: "global-model", SettingPelicanPrompt: "global drawing",
		SettingPelicanReasoningEffort: "high", SettingPelicanParallelCount: "2", SettingPelicanCronExpression: "0 * * * *",
		SettingPelicanActiveTimezone: "America/New_York", SettingPelicanActiveStart: "09:00", SettingPelicanActiveEnd: "18:00",
		SettingPelicanActiveWeekdays: "[1,2,3,4,5]",
	}
	for key, value := range values {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(key, value))
	}
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanPlans, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	expectMissingPelicanSwitches(mock)
	got, err := NewSettings(repo.db).PelicanSettings()
	if err != nil || got.Model != "global-model" || got.Prompt != "global drawing" || got.ReasoningEffort != "high" || got.ParallelCount != 2 || got.CronExpression != "0 * * * *" || got.ActiveTimezone != "America/New_York" || got.ActiveStart != "09:00" || got.ActiveEnd != "18:00" || len(got.ActiveWeekdays) != 5 {
		t.Fatalf("settings=%#v, err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanSettingsReadPersistedPlansOverLegacyAliases(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.MatchExpectationsInOrder(false)
	values := map[string]string{
		SettingPelicanModel: "legacy-model", SettingPelicanPrompt: "legacy prompt", SettingPelicanReasoningEffort: "high",
		SettingPelicanParallelCount: "8", SettingPelicanCronExpression: "0 * * * *", SettingPelicanActiveTimezone: "Asia/Shanghai", SettingPelicanActiveStart: "08:00", SettingPelicanActiveEnd: "22:00",
		SettingPelicanActiveWeekdays: "[0,1,2,3,4,5,6]",
		SettingPelicanPlans:          `[{"model":"first-model,fallback-model","prompt":"first prompt","reasoning_effort":"low"},{"model":"second-model","prompt":"second prompt","reasoning_effort":"high"}]`,
		SettingPelicanPageEnabled:    "true", SettingPelicanTasksEnabled: "true",
		SettingPelicanHistoryCount: "24",
	}
	for key, value := range values {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(key, value))
	}
	got, err := NewSettings(repo.db).PelicanSettings()
	if err != nil || got.HistoryCount != 24 || len(got.Plans) != 2 || got.Model != "first-model,fallback-model" || got.Prompt != "first prompt" || got.ReasoningEffort != "low" || got.ParallelCount != 8 {
		t.Fatalf("settings=%#v err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanMigrationKeepsExistingGlobalConfiguration(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanModel, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(SettingPelicanModel, "global-model"))
	mock.ExpectCommit()
	if err := NewSettings(repo.db).MigratePelicanSettings(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanMigrationUsesMostRecentlySavedLegacyConfiguration(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanModel, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectQuery(`SELECT .*relay_pelican_configs.*ORDER BY updated_at DESC, id DESC`).WillReturnRows(sqlmock.NewRows([]string{"model", "prompt", "reasoning_effort", "parallel_count", "cron_expression"}).AddRow("legacy-model", "legacy drawing", "high", 2, "0 * * * *"))
	for _, setting := range [][2]string{{SettingPelicanModel, "legacy-model"}, {SettingPelicanPrompt, "legacy drawing"}, {SettingPelicanReasoningEffort, "high"}, {SettingPelicanParallelCount, "2"}, {SettingPelicanCronExpression, "0 * * * *"}, {SettingPelicanPlans, `[{"model":"legacy-model","prompt":"legacy drawing","reasoning_effort":"high"}]`}} {
		expectPelicanSettingWrite(mock, setting[0], setting[1])
	}
	mock.ExpectExec(`UPDATE "relay_pelican_configs" SET .* WHERE cron_expression <>`).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()
	if err := NewSettings(repo.db).MigratePelicanSettings(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveGlobalPelicanSettingsReschedulesAccountsWithoutChangingSwitchesOrLeases(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.MatchExpectationsInOrder(false)
	settings := defaultUISettings()
	settings.PelicanCronExpression = "*/15 * * * *"
	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	for key, value := range map[string]string{SettingPelicanActiveTimezone: "Asia/Shanghai", SettingPelicanActiveStart: "08:00", SettingPelicanActiveEnd: "22:00", SettingPelicanActiveWeekdays: "[0,1,2,3,4,5,6]"} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(key, value))
	}
	for key, value := range values {
		expectPelicanSettingWrite(mock, key, value)
	}
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "relay_pelican_configs" SET "cron_expression"=$1,"next_run_at"=$2,"updated_at"=$3 WHERE cron_expression <> $4`)).WithArgs(settings.PelicanCronExpression, sqlmock.AnyArg(), sqlmock.AnyArg(), settings.PelicanCronExpression).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "relay_group_pelican_configs" SET "cron_expression"=$1,"next_run_at"=$2,"updated_at"=$3 WHERE cron_expression <> $4`)).WithArgs(settings.PelicanCronExpression, sqlmock.AnyArg(), sqlmock.AnyArg(), settings.PelicanCronExpression).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	if err := NewSettings(repo.db).SaveUISettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavePelicanHistoryCountLowerDeletesOlderResultsImmediately(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.MatchExpectationsInOrder(false)
	settings := defaultUISettings()
	settings.PelicanHistoryCount = 10
	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(SettingPelicanHistoryCount, "30"))
	for key, value := range map[string]string{SettingPelicanActiveTimezone: "Asia/Shanghai", SettingPelicanActiveStart: "08:00", SettingPelicanActiveEnd: "22:00", SettingPelicanActiveWeekdays: "[0,1,2,3,4,5,6]"} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(key, value))
	}
	for key, value := range values {
		expectPelicanSettingWrite(mock, key, value)
	}
	mock.ExpectExec(`(?s)DELETE FROM relay_pelican_results WHERE id IN .*row_number > \$1`).WithArgs(10).WillReturnResult(sqlmock.NewResult(0, 20))
	mock.ExpectExec(`(?s)DELETE FROM relay_group_pelican_results WHERE id IN .*row_number > \$1`).WithArgs(10).WillReturnResult(sqlmock.NewResult(0, 20))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "relay_pelican_configs" SET "cron_expression"=$1,"next_run_at"=$2,"updated_at"=$3 WHERE cron_expression <> $4`)).WithArgs(settings.PelicanCronExpression, sqlmock.AnyArg(), sqlmock.AnyArg(), settings.PelicanCronExpression).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "relay_group_pelican_configs" SET "cron_expression"=$1,"next_run_at"=$2,"updated_at"=$3 WHERE cron_expression <> $4`)).WithArgs(settings.PelicanCronExpression, sqlmock.AnyArg(), sqlmock.AnyArg(), settings.PelicanCronExpression).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := NewSettings(repo.db).SaveUISettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanAccountJSONContainsNoGenerationSettings(t *testing.T) {
	data, err := json.Marshal(PelicanConfig{Enabled: true, Model: "legacy-model", Prompt: "legacy", ParallelCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"model", "prompt", "reasoning_effort", "parallel_count", "cron_expression"} {
		if _, exists := fields[key]; exists {
			t.Errorf("account JSON must not expose generation setting %s: %s", key, strconv.Quote(string(data)))
		}
	}
}

func TestSavePelicanWindowReschedulesAllAccountsWithoutChangingSwitchesOrLeases(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.MatchExpectationsInOrder(false)
	settings := defaultUISettings()
	settings.PelicanActiveStart, settings.PelicanActiveEnd = "09:00", "18:00"
	settings.PelicanActiveWeekdays = []int{1, 2, 3, 4, 5}
	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	for key, value := range map[string]string{SettingPelicanActiveTimezone: "Asia/Shanghai", SettingPelicanActiveStart: "08:00", SettingPelicanActiveEnd: "22:00", SettingPelicanActiveWeekdays: "[0,1,2,3,4,5,6]"} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(key, value))
	}
	for key, value := range values {
		expectPelicanSettingWrite(mock, key, value)
	}
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "relay_pelican_configs" SET "cron_expression"=$1,"next_run_at"=$2,"updated_at"=$3 WHERE 1 = 1`)).WithArgs(settings.PelicanCronExpression, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "relay_group_pelican_configs" SET "cron_expression"=$1,"next_run_at"=$2,"updated_at"=$3 WHERE 1 = 1`)).WithArgs(settings.PelicanCronExpression, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()
	if err := NewSettings(repo.db).SaveUISettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPelicanMissingWindowSettingsUseDaytimeDefaults(t *testing.T) {
	repo, mock := pelicanDB(t)
	for _, key := range []string{SettingPelicanModel, SettingPelicanPrompt, SettingPelicanReasoningEffort, SettingPelicanCronExpression, SettingPelicanActiveTimezone, SettingPelicanActiveStart, SettingPelicanActiveEnd, SettingPelicanActiveWeekdays, SettingPelicanParallelCount} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	}
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanPlans, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(SettingPelicanHistoryCount, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	expectMissingPelicanSwitches(mock)
	settings, err := NewSettings(repo.db).PelicanSettings()
	if err != nil || settings.ActiveTimezone != "Asia/Shanghai" || settings.ActiveStart != "08:00" || settings.ActiveEnd != "22:00" || len(settings.ActiveWeekdays) != 7 {
		t.Fatalf("settings=%#v err=%v", settings, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
