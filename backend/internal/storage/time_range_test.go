package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTimeRangeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "time-range.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&OperationLedgerEntry{}, &Channel{}, &RelayStation{}, &ChannelUsageEvent{}, &BalanceSnapshot{}, &ChannelDailyBalance{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestOperationQueriesUseHalfOpenCustomWindow(t *testing.T) {
	db := newTimeRangeTestDB(t)
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	entries := []OperationLedgerEntry{
		{Direction: "expense", Category: "account_purchase", Amount: 10, Description: "at start", OccurredAt: start},
		{Direction: "expense", Category: "upstream_recharge", Amount: 20, Description: "inside", OccurredAt: end.Add(-time.Nanosecond)},
		{Direction: "income", Category: "manual_income", Amount: 30, Description: "at end", OccurredAt: end},
		{Direction: "expense", Category: "account_purchase", Amount: 40, Description: "before", OccurredAt: start.Add(-time.Nanosecond)},
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	operations := NewOperations(db)
	summary, _, err := operations.LedgerSummary(start, end)
	if err != nil {
		t.Fatal(err)
	}
	if summary.EntryCount != 2 || summary.ExpenseAmount != 30 {
		t.Fatalf("summary = %#v, want 2 entries and 30 expense", summary)
	}
	list, err := operations.ListLedger(LedgerFilter{Since: start, Until: end})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ledger rows = %d, want 2", len(list))
	}
}

func TestChannelUsageTotalsUseHalfOpenCustomWindow(t *testing.T) {
	db := newTimeRangeTestDB(t)
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	events := []ChannelUsageEvent{
		{ChannelID: 1, ChannelAccountID: 1, Source: "newapi", RemoteUsageID: "start", ActualCost: 1, OccurredAt: start, SyncedAt: start},
		{ChannelID: 1, ChannelAccountID: 1, Source: "newapi", RemoteUsageID: "inside", ActualCost: 2, OccurredAt: end.Add(-time.Nanosecond), SyncedAt: end},
		{ChannelID: 1, ChannelAccountID: 1, Source: "newapi", RemoteUsageID: "end", ActualCost: 4, OccurredAt: end, SyncedAt: end},
		{ChannelID: 1, ChannelAccountID: 1, Source: "newapi", RemoteUsageID: "before", ActualCost: 8, OccurredAt: start.Add(-time.Nanosecond), SyncedAt: start},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatal(err)
	}

	amounts, requests, err := NewChannelUsage(db).CostTotals(start, end)
	if err != nil {
		t.Fatal(err)
	}
	if amounts[1] != 3 || requests[1] != 2 {
		t.Fatalf("custom usage = amount:%v requests:%d, want amount:3 requests:2", amounts[1], requests[1])
	}
}

func TestBalanceRangeBoundsUseExactStartAndHistoricalEnd(t *testing.T) {
	db := newTimeRangeTestDB(t)
	start := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	current := 20.0
	channel := Channel{Name: "history", Type: "newapi", SiteURL: "https://example.com", Username: "test", PasswordCipher: "test", LastBalance: &current}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	snapshots := []BalanceSnapshot{
		{ChannelID: channel.ID, Balance: 100, SampledAt: start.Add(-10 * time.Hour)},
		{ChannelID: channel.ID, Balance: 80, SampledAt: start.Add(-time.Minute)},
		{ChannelID: channel.ID, Balance: 70, SampledAt: end.Add(-time.Minute)},
		{ChannelID: channel.ID, Balance: 40, SampledAt: end},
	}
	if err := db.Create(&snapshots).Error; err != nil {
		t.Fatal(err)
	}
	bounds, err := NewRates(db).rangeBalanceBounds(start, end)
	if err != nil {
		t.Fatal(err)
	}
	bound := bounds[channel.ID]
	if bound.Start == nil || bound.End == nil || *bound.Start != 80 || *bound.End != 70 {
		t.Fatalf("bounds = %#v, want start:80 end:70", bound)
	}
}
