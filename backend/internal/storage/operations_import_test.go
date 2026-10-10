package storage

import "testing"

func TestLedgerSourceBalanceAdjustmentPolicy(t *testing.T) {
	if !shouldApplyRechargeAdjustment(LedgerSourceManual) {
		t.Fatal("manual ledger should adjust manual balance")
	}
	if shouldApplyRechargeAdjustment(LedgerSourceHistoryImport) {
		t.Fatal("historical import must not adjust manual balance")
	}
}

func TestLedgerSourceEditPolicy(t *testing.T) {
	if !canEditLedgerSource(LedgerSourceManual) {
		t.Fatal("manual ledger should be editable")
	}
	if !canEditLedgerSource(LedgerSourceHistoryImport) {
		t.Fatal("historical import ledger should be editable")
	}
	if canEditLedgerSource(LedgerSourceLocalAccount) {
		t.Fatal("local account ledger should be managed by local account pool")
	}
}

func TestLedgerSearchPatternTrimsAndWrapsQuery(t *testing.T) {
	if got := ledgerSearchPattern("  recharge  "); got != "%recharge%" {
		t.Fatalf("search pattern = %q, want %q", got, "%recharge%")
	}
	if got := ledgerSearchPattern("   "); got != "" {
		t.Fatalf("empty search pattern = %q, want empty", got)
	}
}
