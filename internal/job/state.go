package job

// State is the durable state of a job.
//
// State values are part of the persistence and event contracts. Keep them
// lowercase and do not rename them without a data migration.
type State string

const (
	StateAccepted     State = "accepted"
	StateQueued       State = "queued"
	StateLeased       State = "leased"
	StateRunning      State = "running"
	StateRetryWait    State = "retry_wait"
	StateSucceeded    State = "succeeded"
	StateRejected     State = "rejected"
	StateFailed       State = "failed"
	StateCanceled     State = "canceled"
	StateDeadLettered State = "dead_lettered"
)

var transitions = map[State]map[State]struct{}{
	StateAccepted: {
		StateQueued:   {},
		StateCanceled: {},
	},
	StateQueued: {
		StateLeased:   {},
		StateCanceled: {},
	},
	StateLeased: {
		StateRunning:      {},
		StateQueued:       {},
		StateCanceled:     {},
		StateDeadLettered: {},
	},
	StateRunning: {
		StateQueued:       {},
		StateRetryWait:    {},
		StateSucceeded:    {},
		StateRejected:     {},
		StateFailed:       {},
		StateCanceled:     {},
		StateDeadLettered: {},
	},
	StateRetryWait: {
		StateQueued:       {},
		StateCanceled:     {},
		StateDeadLettered: {},
	},
}

// Valid reports whether s is a known durable state.
func (s State) Valid() bool {
	switch s {
	case StateAccepted, StateQueued, StateLeased, StateRunning, StateRetryWait,
		StateSucceeded, StateRejected, StateFailed, StateCanceled, StateDeadLettered:
		return true
	default:
		return false
	}
}

// Terminal reports whether no automatic transition may leave s.
func (s State) Terminal() bool {
	switch s {
	case StateSucceeded, StateRejected, StateFailed, StateCanceled, StateDeadLettered:
		return true
	default:
		return false
	}
}

// CanTransition reports whether a state transition is part of the durable job
// state machine. A transition to the same state is not considered a state
// transition; command-level idempotency is handled separately.
func CanTransition(from, to State) bool {
	_, ok := transitions[from][to]
	return ok
}

// AttemptState is the append-only outcome state of one execution attempt.
type AttemptState string

const (
	AttemptLeased       AttemptState = "leased"
	AttemptRunning      AttemptState = "running"
	AttemptSucceeded    AttemptState = "succeeded"
	AttemptRejected     AttemptState = "rejected"
	AttemptFailed       AttemptState = "failed"
	AttemptCanceled     AttemptState = "canceled"
	AttemptLeaseExpired AttemptState = "lease_expired"
)

// Terminal reports whether the attempt can no longer be executed or renewed.
func (s AttemptState) Terminal() bool {
	switch s {
	case AttemptSucceeded, AttemptRejected, AttemptFailed, AttemptCanceled, AttemptLeaseExpired:
		return true
	default:
		return false
	}
}
