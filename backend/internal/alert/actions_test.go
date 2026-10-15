package alert

import (
	"context"
	"testing"
)

type fakeActions struct {
	accountCalls []string
	userCalls    []string
}

func (f *fakeActions) SetSchedulable(_ context.Context, stationID uint, accountID int64, value bool) error {
	f.accountCalls = append(f.accountCalls, actionKey(stationID, accountID, value))
	return nil
}

func (f *fakeActions) UpdateUserStatus(_ context.Context, stationID uint, userID int64, status string) error {
	f.userCalls = append(f.userCalls, userActionKey(stationID, userID, status))
	return nil
}

func TestAutoDisableIsIdempotent(t *testing.T) {
	fake := &fakeActions{}
	request := ActionRequest{Kind: ActionDisableAccountScheduling, StationID: 7, TargetExternalID: "42"}
	if err := ApplyAction(context.Background(), fake, request); err != nil {
		t.Fatal(err)
	}
	if err := ApplyAction(context.Background(), fake, request); err != nil {
		t.Fatal(err)
	}
	if len(fake.accountCalls) != 2 {
		t.Fatalf("calls = %#v", fake.accountCalls)
	}
}

func TestAutoRecoveryDoesNotOverrideManualChange(t *testing.T) {
	if ShouldAutoRecover(false, false, true) {
		t.Fatal("manual intervention must block recovery")
	}
	if ShouldAutoRecover(false, true, true) {
		t.Fatal("current state change must block recovery")
	}
	if !ShouldAutoRecover(false, false, false) {
		t.Fatal("unchanged automatically applied state should recover")
	}
}
