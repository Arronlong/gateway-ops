package alert

import (
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
