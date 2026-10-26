package storage

import (
	"encoding/json"
	"testing"
)

func TestConfiguredPelicanPlansSkipDisabledAndKeepLegacyEnabled(t *testing.T) {
	var plans []PelicanTestPlan
	if err := json.Unmarshal([]byte(`[{"model":"legacy-model","prompt":"legacy","reasoning_effort":"medium"},{"model":"disabled-model","prompt":"disabled","reasoning_effort":"high","enabled":false},{"model":"enabled-model","prompt":"enabled","reasoning_effort":"low","enabled":true}]`), &plans); err != nil {
		t.Fatal(err)
	}
	got := ConfiguredPelicanPlans(plans)
	if len(got) != 2 || got[0].Model != "legacy-model" || got[1].Model != "enabled-model" {
		t.Fatalf("enabled plans = %#v", got)
	}
	encoded, err := json.Marshal(plans)
	if err != nil {
		t.Fatal(err)
	}
	var restored []PelicanTestPlan
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if len(ConfiguredPelicanPlans(restored)) != 2 {
		t.Fatal("plan switch lost after persistence round trip")
	}
	if len(ConfiguredPelicanPlans(plans[1:2])) != 0 {
		t.Fatal("all-disabled plans must not execute")
	}
}
