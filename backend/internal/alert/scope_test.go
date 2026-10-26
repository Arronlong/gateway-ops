package alert

import "testing"

func TestScopeMatchesAllChannelFilters(t *testing.T) {
	scope := ScopeConfig{Logic: "all", Filters: []ScopeFilter{
		{Field: "name", Operator: "contains", Value: "openai"},
		{Field: "score", Operator: "gte", Value: "80"},
		{Field: "cache_hit_rate", Operator: "gt", Value: "60"},
		{Field: "monitor_enabled", Operator: "eq", Value: "true"},
	}}
	attributes := map[string]string{"name": "Main OpenAI", "score": "92", "cache_hit_rate": "74.5", "monitor_enabled": "true"}
	if !scopeMatches(TargetChannel, scope, attributes) {
		t.Fatal("all matching channel filters should match")
	}
	attributes["score"] = "79"
	if scopeMatches(TargetChannel, scope, attributes) {
		t.Fatal("a failed all-filter must exclude the target")
	}
}

func TestScopeMatchesAnyRelayAccountFilter(t *testing.T) {
	scope := ScopeConfig{Logic: "any", Filters: []ScopeFilter{
		{Field: "station_type", Operator: "eq", Value: "newapi"},
		{Field: "schedulable", Operator: "eq", Value: "false"},
	}}
	if err := validateScope(TargetRelayAccount, scope); err != nil {
		t.Fatal(err)
	}
	if !scopeMatches(TargetRelayAccount, scope, map[string]string{"station_type": "newapi", "schedulable": "true"}) {
		t.Fatal("one matching any-filter should match")
	}
}

func TestValidateScopeRejectsUnsupportedFieldsAndOperators(t *testing.T) {
	for _, scope := range []ScopeConfig{
		{Logic: "all", Filters: []ScopeFilter{{Field: "password", Operator: "eq", Value: "secret"}}},
		{Logic: "all", Filters: []ScopeFilter{{Field: "name", Operator: "regex", Value: ".*"}}},
		{Logic: "xor", Filters: []ScopeFilter{{Field: "name", Operator: "eq", Value: "example"}}},
	} {
		if err := validateScope(TargetRelayAccount, scope); err == nil {
			t.Fatalf("expected invalid scope to be rejected: %+v", scope)
		}
	}
}

func TestScopeFiltersDoNotMatchMissingMetricData(t *testing.T) {
	scope := ScopeConfig{Logic: "all", Filters: []ScopeFilter{{Field: "cache_hit_rate", Operator: "gte", Value: "60"}}}
	if scopeMatches(TargetChannel, scope, map[string]string{"name": "unknown-cache"}) {
		t.Fatal("missing metric data must not satisfy a selector")
	}
}

func TestModelNegativeFiltersExcludeChannelsContainingAnyMatchingModel(t *testing.T) {
	for _, operator := range []string{"ne", "not_contains"} {
		value := "gpt-4.1"
		if operator == "not_contains" {
			value = "gpt"
		}
		scope := ScopeConfig{Filters: []ScopeFilter{{Field: "model", Operator: operator, Value: value}}}
		if scopeMatches(TargetChannel, scope, map[string]string{"model": "claude\ngpt-4.1"}) {
			t.Fatalf("operator %q matched a channel containing a filtered model", operator)
		}
		if !scopeMatches(TargetChannel, scope, map[string]string{"model": "claude\nsonnet"}) {
			t.Fatalf("operator %q did not match a channel without the filtered model", operator)
		}
		if scopeMatches(TargetChannel, scope, map[string]string{"model": ""}) {
			t.Fatalf("operator %q matched when model data is missing", operator)
		}
	}
}
