package job

import "testing"

func TestStateMachine(t *testing.T) {
	t.Parallel()

	all := []State{
		StateAccepted,
		StateQueued,
		StateLeased,
		StateRunning,
		StateRetryWait,
		StateSucceeded,
		StateRejected,
		StateFailed,
		StateCanceled,
		StateDeadLettered,
	}
	for _, state := range all {
		if !state.Valid() {
			t.Fatalf("state %q is not valid", state)
		}
		for _, target := range all {
			if state.Terminal() && CanTransition(state, target) {
				t.Fatalf("terminal state %q transitions to %q", state, target)
			}
		}
	}

	required := [][2]State{
		{StateAccepted, StateQueued},
		{StateAccepted, StateCanceled},
		{StateQueued, StateLeased},
		{StateQueued, StateCanceled},
		{StateLeased, StateRunning},
		{StateLeased, StateQueued},
		{StateRunning, StateRetryWait},
		{StateRunning, StateSucceeded},
		{StateRunning, StateRejected},
		{StateRunning, StateFailed},
		{StateRunning, StateCanceled},
		{StateRunning, StateQueued},
		{StateRetryWait, StateQueued},
	}
	for _, transition := range required {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("required transition %s -> %s is missing", transition[0], transition[1])
		}
	}
	if CanTransition(StateQueued, StateSucceeded) {
		t.Fatal("queued job can skip execution and succeed")
	}
	if CanTransition(StateRunning, StateLeased) {
		t.Fatal("running job can move backward into the same lease")
	}
	if State("DEAD_LETTERED").Valid() {
		t.Fatal("state values must be exact lowercase contract values")
	}
}
