package alert

import (
	"testing"
	"time"
)

func TestStateFiresAfterThreeConsecutiveViolations(t *testing.T) {
	now := time.Now().UTC()
	state := AlertRuleState{State: StateNormal}
	for i := 0; i < 2; i++ {
		transition := Transition(StateInput{Previous: state, Condition: ConditionResult{Violated: true}, TriggerConsecutive: 3, RecoveryConsecutive: 3, Now: now})
		state = transition.Next
		if state.State == StateFiring {
			t.Fatalf("fired too early on violation %d", i+1)
		}
	}
	transition := Transition(StateInput{Previous: state, Condition: ConditionResult{Violated: true}, TriggerConsecutive: 3, RecoveryConsecutive: 3, Now: now})
	if transition.Next.State != StateFiring || !transition.OpenEvent {
		t.Fatalf("expected firing transition with event open, got %+v", transition)
	}
}

func TestStateRecoversAfterThreeHealthyEvaluations(t *testing.T) {
	now := time.Now().UTC()
	state := AlertRuleState{State: StateFiring, ConsecutiveViolations: 3}
	for i := 0; i < 2; i++ {
		transition := Transition(StateInput{Previous: state, Condition: ConditionResult{Violated: false}, TriggerConsecutive: 3, RecoveryConsecutive: 3, Now: now})
		state = transition.Next
		if transition.RecoverEvent {
			t.Fatalf("recovered too early on healthy evaluation %d", i+1)
		}
	}
	transition := Transition(StateInput{Previous: state, Condition: ConditionResult{Violated: false}, TriggerConsecutive: 3, RecoveryConsecutive: 3, Now: now})
	if transition.Next.State != StateRecovered || !transition.RecoverEvent {
		t.Fatalf("expected recovered transition, got %+v", transition)
	}
}

func TestUnknownDataDoesNotFireBusinessAlert(t *testing.T) {
	transition := Transition(StateInput{
		Previous:            AlertRuleState{State: StatePending, ConsecutiveViolations: 2},
		Condition:           ConditionResult{Unknown: true},
		TriggerConsecutive:  3,
		RecoveryConsecutive: 3,
		Now:                 time.Now().UTC(),
	})
	if transition.OpenEvent || transition.Next.State == StateFiring {
		t.Fatalf("unknown data must not fire an alert, got %+v", transition)
	}
}
