package api

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/relay"
	"github.com/guliansheng/gateway-ops/internal/storage"
)

func TestOperationMultiplierUsesActualCostOverRelayConsumption(t *testing.T) {
	value := operationMultiplier(120, 80)
	if value == nil || math.Abs(*value-(80.0/120.0)) > 0.0000001 {
		t.Fatalf("multiplier = %#v, want %v", value, 80.0/120.0)
	}
}

func TestOperationMultiplierReturnsNilWithoutRelayConsumption(t *testing.T) {
	if value := operationMultiplier(0, 80); value != nil {
		t.Fatalf("multiplier = %#v, want nil", value)
	}
}

func TestRelayCostSummaryExcludesUnknownAccountCosts(t *testing.T) {
	knownCost := 40.0
	usage := relay.UsageSummary{UserCharge: 100, FailedAccounts: 1, Accounts: map[int64]relay.AccountUsageStats{
		1: {UserCharge: 60, AccountCost: knownCost, AccountCostKnown: true},
		2: {UserCharge: 40, AccountCostKnown: false},
	}}
	summary := relayCostSummaryFromUsage(usage)
	if summary.Cost != knownCost || summary.UserCharge != 100 || summary.FailedAccounts != 1 || summary.UnknownCostAccounts != 1 || summary.Complete {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestRelayCostSummaryUsesReportedCostForUnknownAccount(t *testing.T) {
	usage := relay.UsageSummary{UserCharge: 100, Accounts: map[int64]relay.AccountUsageStats{
		1: {UserCharge: 60, AccountCost: 40, AccountCostKnown: true},
		2: {UserCharge: 40, AccountCost: 15, AccountCostKnown: false},
	}}
	summary := relayCostSummaryFromUsage(usage)
	if summary.Cost != 55 || summary.UnknownCostAccounts != 1 || summary.Complete {
		t.Fatalf("summary = %#v, want fallback cost 55 with incomplete status", summary)
	}
}

func TestOperationActualCostUsesLedgerExpenses(t *testing.T) {
	if got := operationActualCost(100, 25, 40); got != 85 {
		t.Fatalf("actual consumed cost = %v, want 85", got)
	}
}

func TestOperationActualCostDoesNotBecomeNegativeWhenBalanceExceedsExpenses(t *testing.T) {
	if got := operationActualCost(100, 25, 200); got != 0 {
		t.Fatalf("actual consumed cost = %v, want 0", got)
	}
}

func TestBindLedgerInputAcceptsHistoricalPurchaseRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/operations/ledger", strings.NewReader(`{"direction":"income","category":"purchase_refund","source":"history_import","amount":8.25,"description":"refund","occurred_at":"2026-07-30T16:42:15+08:00"}`))
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req

	entry, err := bindLedgerInput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Direction != "income" || entry.Category != "purchase_refund" || entry.Source != storage.LedgerSourceHistoryImport {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestBindLedgerInputDefaultsManualSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/operations/ledger", strings.NewReader(`{"direction":"expense","category":"upstream_recharge","amount":8.25,"description":"recharge","occurred_at":"2026-07-30T15:42:15+08:00"}`))
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req

	entry, err := bindLedgerInput(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Source != storage.LedgerSourceManual {
		t.Fatalf("source = %q, want %q", entry.Source, storage.LedgerSourceManual)
	}
}
