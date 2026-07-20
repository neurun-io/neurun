package job

import "errors"

var (
	// ErrInvalid is wrapped by validation errors.
	ErrInvalid = errors.New("invalid job command")
	// ErrNotFound deliberately covers both an unknown job and a job in another
	// project so callers cannot use this package to probe project boundaries.
	ErrNotFound = errors.New("job not found")
	// ErrIdempotencyConflict means a project reused an idempotency key for a
	// different immutable request.
	ErrIdempotencyConflict = errors.New("idempotency key used for a different request")
	// ErrInvalidTransition means the command is not legal in the current state.
	ErrInvalidTransition = errors.New("invalid job state transition")
	// ErrNotClaimable means the job is not queued for a new execution attempt.
	ErrNotClaimable = errors.New("job is not claimable")
	// ErrAttemptsExhausted means no further automatic attempt may be created.
	ErrAttemptsExhausted = errors.New("job attempts exhausted")
	// ErrLeaseLost covers an expired, canceled, superseded, or otherwise fenced
	// execution lease. Callers must stop work when they receive it.
	ErrLeaseLost = errors.New("job lease lost")
	// ErrFinalizationConflict means the same attempt was finalized with a
	// different terminal command.
	ErrFinalizationConflict = errors.New("attempt finalization conflict")
	// ErrOutboxClaimLost means a publisher's claim expired or was superseded.
	ErrOutboxClaimLost = errors.New("outbox claim lost")
	// ErrMessageIDConflict means a deterministic message ID was reused with
	// different message content.
	ErrMessageIDConflict = errors.New("message id reused with different content")
	// ErrInvalidCursor means a list cursor was malformed.
	ErrInvalidCursor = errors.New("invalid job list cursor")
)
