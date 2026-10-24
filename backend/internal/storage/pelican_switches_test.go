package storage

import (
	"strconv"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectMissingPelicanSwitches(mock sqlmock.Sqlmock) {
	for _, key := range []string{SettingPelicanPageEnabled, SettingPelicanTasksEnabled} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	}
}

func TestPelicanSwitchesDefaultEnabledAndRemainIndependent(t *testing.T) {
	defaults := defaultUISettings()
	if !defaults.PelicanPageEnabled || !defaults.PelicanTasksEnabled {
		t.Fatal("existing installations must keep both pelican switches enabled")
	}
	for _, page := range []bool{false, true} {
		for _, tasks := range []bool{false, true} {
			settings := defaults
			settings.PelicanPageEnabled, settings.PelicanTasksEnabled = page, tasks
			values, err := uiSettingsValues(settings)
			if err != nil {
				t.Fatal(err)
			}
			if values[SettingPelicanPageEnabled] != strconv.FormatBool(page) || values[SettingPelicanTasksEnabled] != strconv.FormatBool(tasks) {
				t.Fatalf("switches were coupled: %#v", values)
			}
			repo, mock := pelicanDB(t)
			mock.MatchExpectationsInOrder(false)
			for _, key := range []string{SettingPelicanModel, SettingPelicanPrompt, SettingPelicanReasoningEffort, SettingPelicanCronExpression, SettingPelicanActiveTimezone, SettingPelicanActiveStart, SettingPelicanActiveEnd, SettingPelicanActiveWeekdays, SettingPelicanParallelCount, SettingPelicanPlans, SettingPelicanHistoryCount, SettingPelicanPageEnabled, SettingPelicanTasksEnabled} {
				mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(key, values[key]))
			}
			got, err := NewSettings(repo.db).PelicanSettings()
			if err != nil || got.PageEnabled != page || got.TasksEnabled != tasks || got.HistoryCount != PelicanHistoryLimit {
				t.Fatalf("read switches: %#v, err=%v", got, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPelicanMissingSwitchesKeepExistingBehavior(t *testing.T) {
	repo, mock := pelicanDB(t)
	mock.MatchExpectationsInOrder(false)
	for _, key := range []string{SettingPelicanModel, SettingPelicanPrompt, SettingPelicanReasoningEffort, SettingPelicanCronExpression, SettingPelicanActiveTimezone, SettingPelicanActiveStart, SettingPelicanActiveEnd, SettingPelicanActiveWeekdays, SettingPelicanParallelCount, SettingPelicanPlans, SettingPelicanHistoryCount, SettingPelicanPageEnabled, SettingPelicanTasksEnabled} {
		mock.ExpectQuery(`SELECT .*app_settings`).WithArgs(key, 1).WillReturnRows(sqlmock.NewRows([]string{"key", "value"}))
	}
	got, err := NewSettings(repo.db).PelicanSettings()
	if err != nil || !got.PageEnabled || !got.TasksEnabled || got.HistoryCount != PelicanHistoryLimit {
		t.Fatalf("missing switches changed existing behavior: %#v, err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
