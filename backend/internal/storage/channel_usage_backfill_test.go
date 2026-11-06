package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newChannelUsageBackfillTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "usage.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&ChannelUsageEvent{}, &ChannelUsageSyncState{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCompleteUsageBackfillReplacesOnlyTheVerifiedAccountWindow(t *testing.T) {
	db := newChannelUsageBackfillTestDB(t)
	repo := NewChannelUsage(db)
	since := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	until := since.Add(30 * 24 * time.Hour)
	inside := since.Add(time.Hour)
	events := []ChannelUsageEvent{
		{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "1", ActualCost: 40, OccurredAt: inside},
		{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "newapi:v2:retained", ActualCost: 9, OccurredAt: inside},
		{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "2", OccurredAt: since},
		{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "before", OccurredAt: since.Add(-time.Second)},
		{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "at-end", OccurredAt: until},
		{ChannelID: 1, ChannelAccountID: 3, Source: "newapi", RemoteUsageID: "other-account", OccurredAt: inside},
		{ChannelID: 4, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "other-channel", OccurredAt: inside},
		{ChannelID: 1, ChannelAccountID: 2, Source: "sub2api", RemoteUsageID: "other-source", OccurredAt: inside},
	}
	if err := repo.Upsert(events); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		state := &ChannelUsageSyncState{ChannelID: 1, ChannelAccountID: 2, BackfillVersion: 2, ForwardSyncedAt: &until, BackfilledAt: &until}
		replacement := []ChannelUsageEvent{{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "newapi:v2:stable", ActualCost: 44.837808, OccurredAt: inside}}
		if err := repo.CompleteBackfill(state, "newapi", since, until, replacement); err != nil {
			t.Fatal(err)
		}
		var stored []ChannelUsageEvent
		if err := db.Find(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if len(stored) != 8 {
			t.Fatalf("backfill attempt %d stored %d events, want 8", attempt, len(stored))
		}
		found := map[string]bool{}
		for _, event := range stored {
			found[event.RemoteUsageID] = true
		}
		for _, id := range []string{"before", "at-end", "other-account", "other-channel", "other-source", "newapi:v2:stable", "newapi:v2:retained", "2"} {
			if !found[id] {
				t.Fatalf("backfill lost event %s", id)
			}
		}
		var total float64
		if err := db.Model(&ChannelUsageEvent{}).Select("SUM(actual_cost)").Scan(&total).Error; err != nil {
			t.Fatal(err)
		}
		if total != 53.837808 {
			t.Fatalf("cost = %f, want authoritative cost without legacy duplicates", total)
		}
	}
	state, err := repo.SyncState(1, 2)
	if err != nil || state.BackfillVersion != 2 || state.ForwardSyncedAt == nil || !state.ForwardSyncedAt.Equal(until) {
		t.Fatalf("backfill state = %#v, error = %v", state, err)
	}
}

func TestEmptyUsageBackfillPreservesRetainedHistory(t *testing.T) {
	db := newChannelUsageBackfillTestDB(t)
	repo := NewChannelUsage(db)
	since := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	until := since.Add(30 * 24 * time.Hour)
	if err := repo.Upsert([]ChannelUsageEvent{{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "1", OccurredAt: since}}); err != nil {
		t.Fatal(err)
	}
	state := &ChannelUsageSyncState{ChannelID: 1, ChannelAccountID: 2, BackfillVersion: 2}
	if err := repo.CompleteBackfill(state, "newapi", since, until, nil); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&ChannelUsageEvent{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("empty upstream deleted retained history: count=%d, error=%v", count, err)
	}
}

func TestReplaceHistoryClearsStaleRowsOnlyInsideVerifiedWindow(t *testing.T) {
	db := newChannelUsageBackfillTestDB(t)
	repo := NewChannelUsage(db)
	since := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	for index, stamp := range []time.Time{since.Add(-time.Hour), since.Add(time.Hour), since.Add(12 * time.Hour), until} {
		event := ChannelUsageEvent{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: stamp.String(), OccurredAt: stamp}
		if index == 2 {
			event.ChannelAccountID = 3
		}
		if err := repo.Upsert([]ChannelUsageEvent{event}); err != nil {
			t.Fatal(err)
		}
	}
	state := &ChannelUsageSyncState{ChannelID: 1, ChannelAccountID: 2}
	if err := repo.ReplaceHistory(state, "newapi", since, until, []ChannelUsageEvent{{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "replacement", OccurredAt: since.Add(18 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	count, err := repo.CountWindow(1, 2, "newapi", since, until)
	if err != nil || count != 1 {
		t.Fatalf("stale earlier rows survived: count=%d error=%v", count, err)
	}
	var all int64
	if err := db.Model(&ChannelUsageEvent{}).Count(&all).Error; err != nil || all != 4 {
		t.Fatalf("unrelated history changed: count=%d error=%v", all, err)
	}
}

func TestCompleteUsageBackfillRollsBackWhenStateCannotBeSaved(t *testing.T) {
	db := newChannelUsageBackfillTestDB(t)
	repo := NewChannelUsage(db)
	since := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	until := since.Add(30 * 24 * time.Hour)
	state := &ChannelUsageSyncState{ChannelID: 1, ChannelAccountID: 2}
	if err := repo.SaveSyncState(state); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert([]ChannelUsageEvent{{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "1", OccurredAt: since}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER reject_usage_state BEFORE UPDATE ON channel_usage_sync_states BEGIN SELECT RAISE(ABORT, 'state unavailable'); END;").Error; err != nil {
		t.Fatal(err)
	}
	state.BackfillVersion = 2
	replacement := []ChannelUsageEvent{{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", RemoteUsageID: "newapi:v2:new", OccurredAt: since}}
	if err := repo.CompleteBackfill(state, "newapi", since, until, replacement); err == nil {
		t.Fatal("backfill succeeded despite state write failure")
	}
	var stored []ChannelUsageEvent
	if err := db.Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].RemoteUsageID != "1" {
		t.Fatalf("failed backfill changed original records: %#v", stored)
	}
	state, err := repo.SyncState(1, 2)
	if err != nil || state.BackfillVersion != 0 {
		t.Fatalf("failed backfill advanced state: %#v, %v", state, err)
	}
}

func TestCompleteUsageBackfillRejectsEventsOutsideAccountOrWindow(t *testing.T) {
	db := newChannelUsageBackfillTestDB(t)
	repo := NewChannelUsage(db)
	since := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	until := since.Add(30 * 24 * time.Hour)
	state := &ChannelUsageSyncState{ChannelID: 1, ChannelAccountID: 2}
	for _, event := range []ChannelUsageEvent{
		{ChannelID: 1, ChannelAccountID: 3, Source: "newapi", OccurredAt: since},
		{ChannelID: 4, ChannelAccountID: 2, Source: "newapi", OccurredAt: since},
		{ChannelID: 1, ChannelAccountID: 2, Source: "sub2api", OccurredAt: since},
		{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", OccurredAt: since.Add(-time.Second)},
		{ChannelID: 1, ChannelAccountID: 2, Source: "newapi", OccurredAt: until},
	} {
		if err := repo.CompleteBackfill(state, "newapi", since, until, []ChannelUsageEvent{event}); err == nil {
			t.Fatalf("backfill accepted invalid event: %#v", event)
		}
	}
}
