package alert

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestCollectAccountSpendByWindow(t *testing.T) {
	now := time.Now().UTC()
	events := []storage.RelayUsageEvent{
		{ActualCost: 2.5, OccurredAt: now.Add(-2 * time.Minute), StatusCode: 200},
		{ActualCost: 3.5, OccurredAt: now.Add(-time.Minute), StatusCode: 200},
	}
	observation := AggregateUsage(events, now)
	if observation.Value != 6 || observation.RequestCount != 2 {
		t.Fatalf("observation = %+v", observation)
	}
}

func TestCollectChannelBalanceAppliesDynamicScopeAndSkipsDisabledMonitor(t *testing.T) {
	_, repo, fixture, _ := workflowFixture(t)
	db := fixture.db
	if err := db.AutoMigrate(&storage.Channel{}, &storage.ChannelUsageEvent{}, &storage.RateSnapshot{}, &storage.RateChangeLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	balance := 4.0
	observedAt := now.Add(-time.Minute)
	channels := storage.NewChannels(db)
	active := storage.Channel{Name: "prod-openai", Type: "sub2api", SiteURL: "https://channel.example", Username: "ops", PasswordCipher: "cipher", LastBalance: &balance, LastBalanceAt: &observedAt, MonitorEnabled: true}
	disabled := storage.Channel{Name: "prod-disabled", Type: "sub2api", SiteURL: "https://disabled.example", Username: "ops", PasswordCipher: "cipher", LastBalance: &balance, LastBalanceAt: &observedAt, MonitorEnabled: false}
	if err := db.Create(&active).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&disabled).Update("monitor_enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal(ScopeConfig{Logic: "all", Filters: []ScopeFilter{{Field: "name", Operator: "contains", Value: "prod"}, {Field: "monitor_enabled", Operator: "eq", Value: "true"}}})
	rule := storage.AlertRule{ID: 4, TargetType: string(TargetChannel), Metric: string(MetricBalanceLow), ScopeJSON: string(scope)}
	if err := repo.SaveState(&storage.AlertRuleState{RuleID: 4, TargetType: string(TargetChannel), TargetExternalID: "2", State: storage.AlertStateFiring}); err != nil {
		t.Fatal(err)
	}
	collector := NewCollector(repo, storage.NewRelayStations(db), channels)
	targets, err := collector.Collect(context.Background(), rule, ConditionConfig{WindowSeconds: 900}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("channel balance targets = %+v", targets)
	}
	byID := map[string]TargetObservation{}
	for _, target := range targets {
		byID[target.TargetExternalID] = target
	}
	if byID["1"].Observation.Value != balance || !byID["1"].Observation.Valid || byID["2"].Observation.Valid {
		t.Fatalf("disabled monitor must remain unknown for an active event: %+v", targets)
	}
}

func TestCollectAccountKeepsTrackingActiveTargetOutsideDynamicScope(t *testing.T) {
	_, repo, fixture, _ := workflowFixture(t)
	db := fixture.db
	if err := db.AutoMigrate(&storage.RelayStation{}, &storage.RelayAccount{}); err != nil {
		t.Fatal(err)
	}
	station := storage.RelayStation{Name: "Sub2API", Type: "sub2api", BaseURL: "https://station.example", APIKeyCipher: "cipher"}
	if err := storage.NewRelayStations(db).Create(&station); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rate, observed := 1.5, now.Add(-time.Minute)
	account := storage.RelayAccount{RelayStationID: station.ID, ExternalID: 42, Name: "paused-account", Status: "active", Schedulable: false, RateMultiplier: &rate, RateObservedAt: &observed}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveState(&storage.AlertRuleState{RuleID: 8, TargetType: string(TargetRelayAccount), RelayStationID: station.ID, TargetExternalID: "42", State: storage.AlertStateFiring}); err != nil {
		t.Fatal(err)
	}
	rule := storage.AlertRule{ID: 8, TargetType: string(TargetRelayAccount), Metric: string(MetricRateChange), ScopeJSON: `{"station_ids":[1],"filters":[{"field":"schedulable","operator":"eq","value":"true"}]}`}
	collector := NewCollector(repo, storage.NewRelayStations(db), nil)
	targets, err := collector.Collect(context.Background(), rule, ConditionConfig{WindowSeconds: 900}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].TargetExternalID != "42" {
		t.Fatalf("active account outside dynamic scope was not tracked: %+v", targets)
	}
}

type fixedUserSnapshots []storage.RelayUserSnapshot

func (snapshots fixedUserSnapshots) AlertUserSnapshots(context.Context, uint) ([]storage.RelayUserSnapshot, error) {
	return snapshots, nil
}

func TestCollectUserBalanceKeepsActiveTargetOutsideDynamicScope(t *testing.T) {
	_, repo, fixture, _ := workflowFixture(t)
	db := fixture.db
	if err := db.AutoMigrate(&storage.RelayStation{}); err != nil {
		t.Fatal(err)
	}
	station := storage.RelayStation{Name: "NewAPI", Type: "newapi", BaseURL: "https://station.example", APIKeyCipher: "cipher"}
	if err := storage.NewRelayStations(db).Create(&station); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshots := fixedUserSnapshots{
		{ID: 42, Username: "low-balance", Status: "active", Balance: 3, ObservedAt: now.Add(-time.Minute)},
		{ID: 43, Username: "disabled", Status: "disabled", Balance: 1, ObservedAt: now.Add(-time.Minute)},
	}
	scope, _ := json.Marshal(ScopeConfig{StationIDs: []uint{station.ID}, Logic: "all", Filters: []ScopeFilter{{Field: "status", Operator: "eq", Value: "active"}}})
	rule := storage.AlertRule{ID: 5, TargetType: string(TargetRelayUser), Metric: string(MetricUserBalanceLow), ScopeJSON: string(scope)}
	if err := repo.SaveState(&storage.AlertRuleState{RuleID: 5, TargetType: string(TargetRelayUser), RelayStationID: station.ID, TargetExternalID: "43", State: storage.AlertStateFiring}); err != nil {
		t.Fatal(err)
	}
	collector := NewCollector(repo, storage.NewRelayStations(db), nil, CollectorSources{Users: snapshots})
	targets, err := collector.Collect(context.Background(), rule, ConditionConfig{WindowSeconds: 900}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("user balance targets = %+v", targets)
	}
	byID := map[string]TargetObservation{}
	for _, target := range targets {
		byID[target.TargetExternalID] = target
	}
	if byID["42"].TargetName != "low-balance" || byID["42"].Observation.Value != 3 || !byID["42"].Observation.Valid || byID["43"].TargetName != "disabled" {
		t.Fatalf("user balance targets = %+v", targets)
	}
}

func TestCollectUserBalanceExcludesPendingTargetOutsideDynamicScope(t *testing.T) {
	_, repo, fixture, _ := workflowFixture(t)
	db := fixture.db
	if err := db.AutoMigrate(&storage.RelayStation{}); err != nil {
		t.Fatal(err)
	}
	station := storage.RelayStation{Name: "Sub2API", Type: "sub2api", BaseURL: "https://station.example", APIKeyCipher: "cipher"}
	if err := storage.NewRelayStations(db).Create(&station); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	snapshots := fixedUserSnapshots{{ID: 42, Username: "promoted-user", Role: "admin", Balance: 1, ObservedAt: now.Add(-time.Minute)}}
	scope, _ := json.Marshal(ScopeConfig{StationIDs: []uint{station.ID}, Filters: []ScopeFilter{{Field: "role", Operator: "eq", Value: "user"}}})
	rule := storage.AlertRule{ID: 9, TargetType: string(TargetRelayUser), Metric: string(MetricUserBalanceLow), ScopeJSON: string(scope)}
	if err := repo.SaveState(&storage.AlertRuleState{RuleID: rule.ID, TargetType: rule.TargetType, RelayStationID: station.ID, TargetExternalID: "42", State: storage.AlertStatePending, ConsecutiveViolations: 1}); err != nil {
		t.Fatal(err)
	}
	collector := NewCollector(repo, storage.NewRelayStations(db), nil, CollectorSources{Users: snapshots})
	targets, err := collector.Collect(context.Background(), rule, ConditionConfig{WindowSeconds: 900}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("pending user no longer matching scope must not be evaluated: %+v", targets)
	}
}

func TestScopeBypassOnlyTracksTriggeredAlerts(t *testing.T) {
	for _, state := range []storage.AlertLifecycleState{storage.AlertStateNormal, storage.AlertStatePending, storage.AlertStateFiring, storage.AlertStateRecovering, storage.AlertStateRecovered, storage.AlertStateUnknown} {
		t.Run(string(state), func(t *testing.T) {
			want := state == storage.AlertStateFiring || state == storage.AlertStateRecovering
			if got := isActiveAlertState(state); got != want {
				t.Fatalf("scope bypass for %s = %t, want %t", state, got, want)
			}
		})
	}
}

func TestDeriveChannelHealthMetricsMatchesDisplayedScoreAvailability(t *testing.T) {
	now := time.Now().UTC()
	firstToken, duration := int64(100), int64(500)
	samples := make([]storage.ChannelUsageSample, 10)
	for index := range samples {
		samples[index] = storage.ChannelUsageSample{
			CreatedAt: now.Add(-time.Minute), SuccessKnown: true, Success: true,
			FirstTokenMS: &firstToken, DurationMS: &duration, InputTokens: 80, CacheReadTokens: 20,
		}
	}
	metrics := deriveChannelMetrics(samples, now)
	if metrics.Score == nil || *metrics.Score != 80 || metrics.CacheHitRate == nil || *metrics.CacheHitRate != 20 {
		t.Fatalf("channel derived metrics = %+v", metrics)
	}
	metrics = deriveChannelMetrics(samples[:9], now)
	if metrics.Score == nil || *metrics.Score != 80 {
		t.Fatalf("low-confidence displayed score must remain filterable: %+v", metrics)
	}
	metrics = deriveChannelMetrics(samples[:1], now)
	if metrics.Score != nil {
		t.Fatalf("score without enough latency variance samples must be unknown: %+v", metrics)
	}
	for index := range samples {
		samples[index].CreatedAt = now.Add(-2 * time.Hour)
	}
	if metrics := deriveChannelMetrics(samples, now); metrics.Score != nil || metrics.CacheHitRate != nil {
		t.Fatalf("stale channel samples must be unknown: %+v", metrics)
	}
}

func TestChannelRateChangeUsesValidModelWhenAnotherBaselineIsZero(t *testing.T) {
	_, _, fixture, _ := workflowFixture(t)
	db := fixture.db
	if err := db.AutoMigrate(&storage.RateSnapshot{}, &storage.RateChangeLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	zero, one := 0.0, 1.0
	if err := db.Create(&[]storage.RateSnapshot{
		{ChannelID: 1, ModelName: "free-to-paid", Ratio: 2, Source: storage.RateSnapshotSourceUpstream},
		{ChannelID: 1, ModelName: "stable-base", Ratio: 1.25, Source: storage.RateSnapshotSourceUpstream},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]storage.RateChangeLog{
		{ChannelID: 1, ModelName: "free-to-paid", OldRatio: &zero, NewRatio: 2, ChangedAt: now.Add(-time.Minute)},
		{ChannelID: 1, ModelName: "stable-base", OldRatio: &one, NewRatio: 1.25, ChangedAt: now.Add(-2 * time.Minute)},
	}).Error; err != nil {
		t.Fatal(err)
	}
	collector := &Collector{rates: storage.NewRates(db)}
	observation, err := collector.channelRateChange(1, ScopeConfig{}, map[string]string{}, ConditionConfig{WindowSeconds: 900, Threshold: 20, RateMode: "percent"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Valid || observation.Value != 25 {
		t.Fatalf("valid model change was masked by zero baseline: %+v", observation)
	}
}

func TestRateCollectionUsesProbeTimestampInsteadOfPollTime(t *testing.T) {
	now := time.Now().UTC()
	old, value := now.Add(-time.Hour), 2.0
	collector := &Collector{}
	account := storage.RelayAccount{RateMultiplier: &value, RateObservedAt: &old, RatePolledAt: &now}
	observation, err := collector.collectAccount(7, account, MetricRateChange, RuleSpec{}, now)
	if err != nil || observation.Valid {
		t.Fatal("successful poll must not make an expired upstream probe fresh")
	}
	current := now.Add(-time.Minute)
	account.RateObservedAt = &current
	observation, err = collector.collectAccount(7, account, MetricRateChange, RuleSpec{}, now)
	if err != nil || !observation.Valid || !observation.ObservedAt.Equal(current) {
		t.Fatal("probe timestamp must identify the sample")
	}
}

func TestUsageWatermarkRequiresCompleteSampleWindow(t *testing.T) {
	_, repo, fixture, _ := workflowFixture(t)
	if err := fixture.db.AutoMigrate(&storage.RelayUsageSyncState{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	forward, backfill := now.Add(-2*time.Minute), now.Add(-15*time.Minute)
	stations := storage.NewRelayStations(fixture.db)
	state := storage.RelayUsageSyncState{RelayStationID: 7, ForwardSyncedAt: &forward, BackfilledAt: &backfill}
	if err := stations.SaveUsageSyncState(state); err != nil {
		t.Fatal(err)
	}
	collector := NewCollector(repo, stations, nil)
	if _, valid := collector.usageWatermark(7, 15*time.Minute, now); valid {
		t.Fatal("watermark window contains an unsynced gap")
	}
	backfill = forward.Add(-15 * time.Minute)
	if err := stations.SaveUsageSyncState(state); err != nil {
		t.Fatal(err)
	}
	if observed, valid := collector.usageWatermark(7, 15*time.Minute, now); !valid || !observed.Equal(forward) {
		t.Fatal("complete window was rejected")
	}
}

func TestCollectUserSpendUsesStableExternalID(t *testing.T) {
	now := time.Now().UTC()
	events := []storage.RelayUsageEvent{
		{UserExternalID: "42", UserEmail: "old@example.com", ActualCost: 4, OccurredAt: now.Add(-time.Minute)},
		{UserExternalID: "99", UserEmail: "old@example.com", ActualCost: 50, OccurredAt: now.Add(-time.Minute)},
	}
	filtered := FilterUsageByUser(events, "42")
	if len(filtered) != 1 || filtered[0].ActualCost != 4 {
		t.Fatalf("filtered = %+v", filtered)
	}
}

func TestCollectStabilityUsesErrorRateAndP95(t *testing.T) {
	now := time.Now().UTC()
	events := []storage.RelayUsageEvent{
		{StatusCode: 200, FirstTokenMS: 100, DurationMS: 300, OccurredAt: now},
		{StatusCode: 500, FirstTokenMS: 200, DurationMS: 900, OccurredAt: now},
		{StatusCode: 200, FirstTokenMS: 300, DurationMS: 700, OccurredAt: now},
	}
	metrics := AggregateStability(events)
	if metrics.RequestCount != 3 || metrics.ErrorCount != 1 || metrics.FirstTokenP95 != 300 || metrics.DurationP95 != 900 {
		t.Fatalf("stability = %+v", metrics)
	}
}

func TestCollectBalanceUsesLowestFreshLinkedChannelBalance(t *testing.T) {
	now := time.Now().UTC()
	balanceA, balanceB := 10.0, 4.0
	timeA, timeB := now.Add(-time.Minute), now.Add(-2*time.Minute)
	accounts := []storage.ChannelAccount{
		{ID: 1, LastBalance: &balanceA, LastBalanceAt: &timeA},
		{ID: 2, LastBalance: &balanceB, LastBalanceAt: &timeB},
	}
	links := []storage.ChannelRelayAccountLink{
		{ChannelAccountID: 1, RelayStationID: 7, RelayAccountExternalID: 42},
		{ChannelAccountID: 2, RelayStationID: 7, RelayAccountExternalID: 42},
	}
	value, ok := LowestFreshBalance(accounts, links, 7, 42, now, 5*time.Minute)
	if !ok || value != 4 {
		t.Fatalf("balance = %v, ok=%v", value, ok)
	}
}

func TestCollectMarksExpiredBalanceUnknown(t *testing.T) {
	now := time.Now().UTC()
	balance := 10.0
	observedAt := now.Add(-10 * time.Minute)
	accounts := []storage.ChannelAccount{{ID: 1, LastBalance: &balance, LastBalanceAt: &observedAt}}
	links := []storage.ChannelRelayAccountLink{{ChannelAccountID: 1, RelayStationID: 7, RelayAccountExternalID: 42}}
	if _, ok := LowestFreshBalance(accounts, links, 7, 42, now, 5*time.Minute); ok {
		t.Fatal("expired balance must be unknown")
	}
}
