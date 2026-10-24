package storage

import (
	"encoding/json"
	"testing"
)

func TestUISettingsOperationsManagementDefaultsEnabled(t *testing.T) {
	t.Parallel()

	got := defaultUISettings()
	if !got.OperationsManagementEnabled {
		t.Fatal("operations management must be enabled by default")
	}
	if got.RecentMetricsWindowHours != 1 {
		t.Fatalf("recent metrics window hours = %d, want 1", got.RecentMetricsWindowHours)
	}
	if got.PelicanHistoryCount != PelicanHistoryLimit {
		t.Fatalf("pelican history count = %d, want %d", got.PelicanHistoryCount, PelicanHistoryLimit)
	}
}

func TestUISettingsPelicanDefaultsAreGlobal(t *testing.T) {
	t.Parallel()

	got := defaultUISettings()
	if got.PelicanModel != "" {
		t.Fatalf("pelican model = %q, want empty until configured", got.PelicanModel)
	}
	if got.PelicanActiveStart != "08:00" || got.PelicanActiveEnd != "22:00" {
		t.Fatalf("unexpected pelican window: %s-%s", got.PelicanActiveStart, got.PelicanActiveEnd)
	}
	if got.PelicanActiveTimezone != "Asia/Shanghai" {
		t.Fatalf("pelican active timezone = %q, want Asia/Shanghai", got.PelicanActiveTimezone)
	}
	for day, gotDay := range got.PelicanActiveWeekdays {
		if day != gotDay {
			t.Fatalf("pelican active weekdays = %v, want all days 0 through 6", got.PelicanActiveWeekdays)
		}
	}
	if len(got.PelicanActiveWeekdays) != 7 {
		t.Fatalf("pelican active weekdays = %v, want all days 0 through 6", got.PelicanActiveWeekdays)
	}
	if got.PelicanPrompt == "" {
		t.Fatal("pelican prompt must have a default")
	}
	if got.PelicanReasoningEffort != "medium" || got.PelicanParallelCount != 1 || got.PelicanCronExpression != "*/30 * * * *" {
		t.Fatalf("unexpected pelican defaults: effort=%q parallel=%d cron=%q", got.PelicanReasoningEffort, got.PelicanParallelCount, got.PelicanCronExpression)
	}
}

func TestUISettingsValuesSerializeGlobalPelicanConfiguration(t *testing.T) {
	t.Parallel()

	settings := defaultUISettings()
	settings.PelicanModel = "gpt-test"
	settings.PelicanPrompt = "draw an animated bicycle"
	settings.PelicanReasoningEffort = "high"
	settings.PelicanParallelCount = 2
	settings.PelicanCronExpression = "0 * * * *"

	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		SettingPelicanModel:           "gpt-test",
		SettingPelicanPrompt:          "draw an animated bicycle",
		SettingPelicanReasoningEffort: "high",
		SettingPelicanParallelCount:   "2",
		SettingPelicanCronExpression:  "0 * * * *",
		SettingPelicanActiveStart:     "08:00",
		SettingPelicanActiveEnd:       "22:00",
		SettingPelicanActiveTimezone:  "Asia/Shanghai",
		SettingPelicanActiveWeekdays:  "[0,1,2,3,4,5,6]",
		SettingPelicanHistoryCount:    "30",
	}
	for key, expected := range want {
		if got := values[key]; got != expected {
			t.Errorf("setting %q = %q, want %q", key, got, expected)
		}
	}
}

func TestUISettingsValuesPersistIndependentPelicanPlans(t *testing.T) {
	settings := defaultUISettings()
	settings.PelicanPlans = []PelicanTestPlan{
		{Model: "model-a,model-a-fallback", Prompt: "draw A", ReasoningEffort: "low"},
		{Model: "model-a", Prompt: "draw B", ReasoningEffort: "high"},
	}
	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	var got []PelicanTestPlan
	if err := json.Unmarshal([]byte(values[SettingPelicanPlans]), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Prompt != "draw A" || got[1].ReasoningEffort != "high" || values[SettingPelicanParallelCount] != "1" {
		t.Fatalf("saved pelican plans = %#v, parallel compatibility setting = %q", got, values[SettingPelicanParallelCount])
	}
}

func TestUISettingsValuesSerializesOperationsManagement(t *testing.T) {
	t.Parallel()

	settings := defaultUISettings()
	settings.OperationsManagementEnabled = false

	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[SettingUIOperationsManagementEnabled]; got != "false" {
		t.Fatalf("operations management value = %q, want false", got)
	}
	if got := values[SettingUIRecentMetricsWindowHours]; got != "1" {
		t.Fatalf("recent metrics window hours value = %q, want 1", got)
	}
}

func TestUISettingsIframeDefaults(t *testing.T) {
	t.Parallel()

	got := defaultUISettings()
	if got.IframeEmbeddingEnabled {
		t.Fatal("iframe embedding must be disabled by default")
	}
	if got.IframeEmbeddingMode != IframeEmbeddingModeAllowlist {
		t.Fatalf("iframe embedding mode = %q, want %q", got.IframeEmbeddingMode, IframeEmbeddingModeAllowlist)
	}
	if len(got.IframeEmbeddingAllowlist) != 0 {
		t.Fatalf("iframe embedding allowlist = %#v, want empty", got.IframeEmbeddingAllowlist)
	}
}

func TestUISettingsValuesSerializesIframeAllowlist(t *testing.T) {
	t.Parallel()

	settings := defaultUISettings()
	settings.IframeEmbeddingEnabled = true
	settings.IframeEmbeddingMode = IframeEmbeddingModeAllowlist
	settings.IframeEmbeddingAllowlist = []string{"*.example.com", "a*abc.com"}

	values, err := uiSettingsValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := values[SettingSecurityIframeEnabled]; got != "true" {
		t.Fatalf("enabled value = %q, want true", got)
	}
	if got := values[SettingSecurityIframeMode]; got != IframeEmbeddingModeAllowlist {
		t.Fatalf("mode value = %q, want %q", got, IframeEmbeddingModeAllowlist)
	}
	if got := values[SettingSecurityIframeAllowlist]; got != `["*.example.com","a*abc.com"]` {
		t.Fatalf("allowlist value = %q", got)
	}
}

func TestDecodeStringSliceSetting(t *testing.T) {
	t.Parallel()

	got, err := decodeStringSliceSetting(`["*.example.com","a*abc.com"]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "*.example.com" || got[1] != "a*abc.com" {
		t.Fatalf("decoded allowlist = %#v", got)
	}
}
