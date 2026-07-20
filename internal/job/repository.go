package job

import (
	"context"
	"encoding/json"
	"time"
)

// AcceptCommand durably accepts an immutable request. IdempotencyKey is scoped
// by Request.ProjectID().
type AcceptCommand struct {
	Request        Request
	IdempotencyKey string
}

type AcceptResult struct {
	Job       Job
	Duplicate bool
}

// ListOptions uses an opaque, stable keyset cursor.
type ListOptions struct {
	States       []State
	CreatedAfter time.Time
	Limit        int
	Cursor       string
}

type JobPage struct {
	Jobs       []Job
	NextCursor string
}

type ClaimOutboxCommand struct {
	Owner string
	Limit int
	TTL   time.Duration
}

// LeaseCommand requests ownership of a queued job. Queue delivery alone is not
// ownership; a caller may execute only after this command succeeds.
type LeaseCommand struct {
	ProjectID string
	JobID     string
	AgentID   string
	TTL       time.Duration
	TraceID   string
}

// Lease is the only value containing the opaque execution token.
type Lease struct {
	Job       Job
	Attempt   Attempt
	Token     string
	Fence     uint64
	ExpiresAt time.Time
}

// LeaseRef identifies an existing lease and supplies both token and fence.
type LeaseRef struct {
	ProjectID string
	JobID     string
	AttemptID string
	Token     string
	Fence     uint64
}

type RenewLeaseCommand struct {
	Lease LeaseRef
	TTL   time.Duration
}

// RetryDirective is a policy decision already made by the execution layer.
// Retryable failure metadata is still required, and the repository enforces
// MaxAttempts.
type RetryDirective struct {
	After  time.Duration
	Reason string
}

// FinalizeCommand commits one attempt outcome. Outcome must be succeeded,
// rejected, failed, or canceled. Repeating the exact command is idempotent;
// changing its content after commit is a conflict.
type FinalizeCommand struct {
	Lease   LeaseRef
	Outcome State
	Result  json.RawMessage
	Failure *Failure
	Retry   *RetryDirective
}

type FinalizeResult struct {
	Job       Job
	Attempt   Attempt
	Duplicate bool
}

type CancelResult struct {
	Job             Job
	Duplicate       bool
	WasRunning      bool
	AgentID         string
	CancellationMsg *Outbox
}

type Recovery struct {
	Job       Job
	Attempt   Attempt
	Requeued  bool
	Exhausted bool
}

// JobReader contains project-scoped query operations.
type JobReader interface {
	Get(context.Context, string, string) (Job, error)
	List(context.Context, string, ListOptions) (JobPage, error)
	Events(context.Context, string, string) ([]Event, error)
	Attempts(context.Context, string, string) ([]Attempt, error)
}

// JobWriter contains aggregate commands that a PostgreSQL adapter must execute
// transactionally.
type JobWriter interface {
	Accept(context.Context, AcceptCommand) (AcceptResult, error)
	Cancel(context.Context, string, string, string) (CancelResult, error)
	EnqueueDueRetries(context.Context, int) ([]Job, error)
}

// LeaseRepository contains compare-and-swap lease operations. PostgreSQL
// implementations should check token hashes, fence, state, and expiry in the
// same statement/transaction that performs each update.
type LeaseRepository interface {
	AcquireLease(context.Context, LeaseCommand) (Lease, error)
	Start(context.Context, LeaseRef) (Attempt, error)
	RenewLease(context.Context, RenewLeaseCommand) (Lease, error)
	Finalize(context.Context, FinalizeCommand) (FinalizeResult, error)
	RecoverExpiredLeases(context.Context, int) ([]Recovery, error)
}

// OutboxRepository is deliberately queue-neutral. Publishing happens outside
// the database transaction; only claims and acknowledgements are persisted.
type OutboxRepository interface {
	ClaimOutbox(context.Context, ClaimOutboxCommand) ([]OutboxClaim, error)
	MarkOutboxPublished(context.Context, string, string) (Outbox, error)
	MarkOutboxFailed(context.Context, string, string, string) (Outbox, error)
}

// Repository is the complete durable job persistence port.
type Repository interface {
	JobReader
	JobWriter
	LeaseRepository
	OutboxRepository
}

// Publisher is the transport port implemented by JetStream in production.
// Implementations must pass Message.ID as the broker deduplication/message ID.
type Publisher interface {
	Publish(context.Context, Message) error
}
