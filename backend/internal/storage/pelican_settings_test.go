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
		settings := defaultUISettings().PelicanSettings()
		update(&settings)
		if err := ValidatePelicanSettings(settings); err == nil {
			t.Fatalf("accepted invalid settings %#v", settings)
		}
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
	}
	for key, value := range values {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(key, value))
	}
	got, err := NewSettings(repo.db).PelicanSettings()
	if err != nil || got.Model != "global-model" || got.Prompt != "global drawing" || got.ReasoningEffort != "high" || got.ParallelCount != 2 || got.CronExpression != "0 * * * *" {
		t.Fatalf("settings=%#v, err=%v", got, err)
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
	for _, setting := range [][2]string{{SettingPelicanModel, "legacy-model"}, {SettingPelicanPrompt, "legacy drawing"}, {SettingPelicanReasoningEffort, "high"}, {SettingPelicanParallelCount, "2"}, {SettingPelicanCronExpression, "0 * * * *"}} {
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
	for key, value := range values {
		expectPelicanSettingWrite(mock, key, value)
	}
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "relay_pelican_configs" SET "cron_expression"=$1,"next_run_at"=$2,"updated_at"=$3 WHERE cron_expression <> $4`)).WithArgs(settings.PelicanCronExpression, sqlmock.AnyArg(), sqlmock.AnyArg(), settings.PelicanCronExpression).WillReturnResult(sqlmock.NewResult(0, 2))
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
