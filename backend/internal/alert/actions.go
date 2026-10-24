package alert

import (
	"context"
	"fmt"
	"strconv"
)

type ActionExecutor interface {
	SetSchedulable(ctx context.Context, stationID uint, accountExternalID int64, schedulable bool) error
	UpdateUserStatus(ctx context.Context, stationID uint, userID int64, status string) error
}

type ActionRequest struct {
	Kind             ActionKind
	StationID        uint
	TargetExternalID string
}

func ApplyAction(ctx context.Context, executor ActionExecutor, request ActionRequest) error {
	switch request.Kind {
	case ActionDisableAccountScheduling:
		accountID, err := strconv.ParseInt(request.TargetExternalID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid account target %q: %w", request.TargetExternalID, err)
		}
		return executor.SetSchedulable(ctx, request.StationID, accountID, false)
	case ActionEnableAccountScheduling:
		accountID, err := strconv.ParseInt(request.TargetExternalID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid account target %q: %w", request.TargetExternalID, err)
		}
		return executor.SetSchedulable(ctx, request.StationID, accountID, true)
	case ActionDisableUser:
		userID, err := strconv.ParseInt(request.TargetExternalID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid user target %q: %w", request.TargetExternalID, err)
		}
		return executor.UpdateUserStatus(ctx, request.StationID, userID, "disabled")
	case ActionEnableUser:
		userID, err := strconv.ParseInt(request.TargetExternalID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid user target %q: %w", request.TargetExternalID, err)
		}
		return executor.UpdateUserStatus(ctx, request.StationID, userID, "active")
	default:
		return fmt.Errorf("unsupported alert action: %s", request.Kind)
	}
}

func ShouldAutoRecover(stateChanged, manualIntervened, recoveryDisabled bool) bool {
	return !stateChanged && !manualIntervened && !recoveryDisabled
}

func actionKey(stationID uint, accountID int64, value bool) string {
	return fmt.Sprintf("%d:%d:%t", stationID, accountID, value)
}

func userActionKey(stationID uint, userID int64, status string) string {
	return fmt.Sprintf("%d:%d:%s", stationID, userID, status)
}
