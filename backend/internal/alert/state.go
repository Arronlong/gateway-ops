package alert

import (
	"time"
)

func Transition(input StateInput) StateTransition {
	state := input.Previous
	if state.State == "" {
		state.State = StateNormal
	}
	if input.TriggerConsecutive <= 0 {
		input.TriggerConsecutive = 1
	}
	if input.RecoveryConsecutive <= 0 {
		input.RecoveryConsecutive = 1
	}
	state.LastEvaluatedAt = &input.Now

	if input.Condition.Unknown {
		state.State = StateUnknown
		return StateTransition{Next: state}
	}

	if input.Condition.Violated {
		state.ConsecutiveRecoveries = 0
		state.ConsecutiveViolations++
		state.LastViolationAt = &input.Now
		if state.State == StateUnknown || state.State == StateRecovered || state.State == StateNormal {
			state.State = StatePending
		}
		if state.ConsecutiveViolations >= input.TriggerConsecutive && state.State != StateFiring {
			state.State = StateFiring
			return StateTransition{Next: state, OpenEvent: true}
		}
		return StateTransition{Next: state}
	}

	state.ConsecutiveViolations = 0
	if state.State == StateFiring || state.State == StateRecovering {
		state.ConsecutiveRecoveries++
		if state.State == StateFiring {
			state.State = StateRecovering
		}
		if state.ConsecutiveRecoveries >= input.RecoveryConsecutive {
			state.State = StateRecovered
			state.LastRecoveredAt = &input.Now
			return StateTransition{Next: state, RecoverEvent: true}
		}
		return StateTransition{Next: state}
	}

	state.ConsecutiveRecoveries = 0
	state.State = StateNormal
	return StateTransition{Next: state}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
