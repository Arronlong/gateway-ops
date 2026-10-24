package alert

import (
	"encoding/json"
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
	active := state.State == StateFiring || state.State == StateRecovering

	if input.Condition.Unknown {
		state.ConsecutiveViolations = 0
		state.ConsecutiveRecoveries = 0
		state.HistoryJSON = "[]"
		state.State = StateUnknown
		if active {
			state.State = StateFiring
		}
		return StateTransition{Next: state}
	}
	if input.ObservedAt != nil {
		if state.LastObservedAt != nil && !input.ObservedAt.After(*state.LastObservedAt) {
			return StateTransition{Next: state}
		}
		state.LastObservedAt = input.ObservedAt
	}
	if !active && input.HitWindow > 0 {
		var history []bool
		_ = json.Unmarshal([]byte(state.HistoryJSON), &history)
		history = append(history, input.Condition.Violated)
		if len(history) > input.HitWindow {
			history = history[len(history)-input.HitWindow:]
		}
		data, _ := json.Marshal(history)
		state.HistoryJSON = string(data)
		count := 0
		for _, hit := range history {
			if hit {
				count++
			}
		}
		state.ConsecutiveViolations = count
		if count >= input.TriggerConsecutive {
			state.State = StateFiring
			return StateTransition{Next: state, OpenEvent: true}
		}
		state.State = StatePending
		if count == 0 {
			state.State = StateNormal
		}
		return StateTransition{Next: state}
	}

	if input.Condition.Violated {
		state.ConsecutiveRecoveries = 0
		state.ConsecutiveViolations++
		state.LastViolationAt = &input.Now
		if active {
			state.State = StateFiring
			return StateTransition{Next: state}
		}
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
	if input.Condition.InRecoveryBand {
		state.ConsecutiveRecoveries = 0
		return StateTransition{Next: state}
	}
	if state.State == StateFiring || state.State == StateRecovering {
		state.ConsecutiveRecoveries++
		if state.State == StateFiring {
			state.State = StateRecovering
		}
		if state.ConsecutiveRecoveries >= input.RecoveryConsecutive {
			state.State = StateRecovered
			state.LastRecoveredAt = &input.Now
			state.HistoryJSON = "[]"
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
