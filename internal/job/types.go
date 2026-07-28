package job

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	// DefaultMaxAttempts is used when a request does not specify an attempt
	// limit.
	DefaultMaxAttempts = 3
	// MaxAttempts is the conservative domain ceiling for one durable job. It
	// bounds attempt history growth even when callers have not applied quotas.
	MaxAttempts           = 10
	defaultInitialBackoff = time.Second
	defaultMaxBackoff     = time.Minute
)

// FunctionRef identifies an immutable function implementation. Aliases must be
// resolved before a Request is constructed.
type FunctionRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// RetryPolicy is copied into the immutable request at acceptance time.
type RetryPolicy struct {
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
}

// Backoff returns a bounded exponential delay for the given one-based attempt.
// Jitter, when desired, belongs in the policy decision layer and can be
// persisted in FinalizeCommand.Retry.After.
func (p RetryPolicy) Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	initial := p.InitialBackoff
	if initial <= 0 {
		initial = defaultInitialBackoff
	}
	maximum := p.MaxBackoff
	if maximum <= 0 {
		maximum = defaultMaxBackoff
	}
	if maximum < initial {
		maximum = initial
	}

	delay := initial
	for n := 1; n < attempt && delay < maximum; n++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

// RequestOptions controls durable retry behavior.
type RequestOptions struct {
	MaxAttempts int
	RetryPolicy RetryPolicy
}

// Request is the immutable execution request persisted with a Job. Its fields
// are private by design. Accessors return values or defensive copies.
type Request struct {
	projectID   string
	function    FunctionRef
	input       json.RawMessage
	maxAttempts int
	retryPolicy RetryPolicy
	digest      string
}

// NewRequest validates and canonicalizes a request. The input JSON and all
// metadata are copied, so subsequent changes to caller-owned buffers cannot
// alter an accepted job.
func NewRequest(projectID string, function FunctionRef, input json.RawMessage, options RequestOptions) (Request, error) {
	if strings.TrimSpace(projectID) == "" {
		return Request{}, fmt.Errorf("%w: project id is required", ErrInvalid)
	}
	if strings.TrimSpace(function.Name) == "" {
		return Request{}, fmt.Errorf("%w: resolved function name is required", ErrInvalid)
	}
	if strings.TrimSpace(function.Version) == "" {
		return Request{}, fmt.Errorf("%w: resolved function version is required", ErrInvalid)
	}
	if strings.TrimSpace(function.Digest) == "" {
		return Request{}, fmt.Errorf("%w: resolved function digest is required", ErrInvalid)
	}

	canonical, err := canonicalJSON(input)
	if err != nil {
		return Request{}, fmt.Errorf("%w: function input: %v", ErrInvalid, err)
	}

	maxAttempts := options.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = DefaultMaxAttempts
	}
	if maxAttempts < 1 {
		return Request{}, fmt.Errorf("%w: max attempts must be at least one", ErrInvalid)
	}
	if maxAttempts > MaxAttempts {
		return Request{}, fmt.Errorf("%w: max attempts cannot exceed %d", ErrInvalid, MaxAttempts)
	}

	policy := options.RetryPolicy
	if policy.InitialBackoff < 0 || policy.MaxBackoff < 0 {
		return Request{}, fmt.Errorf("%w: retry backoff cannot be negative", ErrInvalid)
	}
	if policy.InitialBackoff == 0 {
		policy.InitialBackoff = defaultInitialBackoff
	}
	if policy.MaxBackoff == 0 {
		policy.MaxBackoff = defaultMaxBackoff
	}
	if policy.MaxBackoff < policy.InitialBackoff {
		return Request{}, fmt.Errorf("%w: maximum retry backoff is less than initial backoff", ErrInvalid)
	}

	r := Request{
		projectID:   projectID,
		function:    function,
		input:       canonical,
		maxAttempts: maxAttempts,
		retryPolicy: policy,
	}
	r.digest = digestRequest(r)
	return r, nil
}

func (r Request) ProjectID() string        { return r.projectID }
func (r Request) Function() FunctionRef    { return r.function }
func (r Request) MaxAttempts() int         { return r.maxAttempts }
func (r Request) RetryPolicy() RetryPolicy { return r.retryPolicy }
func (r Request) Digest() string           { return r.digest }

// Input returns a defensive copy of the canonical function input.
func (r Request) Input() json.RawMessage {
	return cloneBytes(r.input)
}

// MarshalJSON exposes the immutable request in persistence- and API-friendly
// form without making its internal byte slice mutable.
func (r Request) MarshalJSON() ([]byte, error) {
	type wireRequest struct {
		ProjectID   string          `json:"project_id"`
		Function    FunctionRef     `json:"function"`
		Input       json.RawMessage `json:"input"`
		MaxAttempts int             `json:"max_attempts"`
		RetryPolicy RetryPolicy     `json:"retry_policy"`
		Digest      string          `json:"digest"`
	}
	return json.Marshal(wireRequest{
		ProjectID:   r.projectID,
		Function:    r.function,
		Input:       r.input,
		MaxAttempts: r.maxAttempts,
		RetryPolicy: r.retryPolicy,
		Digest:      r.digest,
	})
}

// Failure is a classified attempt failure. Details must be bounded and
// redacted by the layer that creates it.
type Failure struct {
	Category  string          `json:"category"`
	Code      string          `json:"code,omitempty"`
	Message   string          `json:"message,omitempty"`
	Retryable bool            `json:"retryable"`
	Ambiguous bool            `json:"ambiguous,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
}

// RetryMetadata records a persisted retry decision.
type RetryMetadata struct {
	Reason          string        `json:"reason"`
	AttemptNumber   int           `json:"attempt_number"`
	MaxAttempts     int           `json:"max_attempts"`
	ScheduledAt     time.Time     `json:"scheduled_at"`
	Backoff         time.Duration `json:"backoff"`
	FailureCategory string        `json:"failure_category,omitempty"`
	Exhausted       bool          `json:"exhausted,omitempty"`
}

// Job is a detached snapshot of the durable aggregate. Mutating a returned Job
// cannot mutate repository state.
type Job struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	IdempotencyKey    string          `json:"-"`
	Request           Request         `json:"request"`
	State             State           `json:"state"`
	AttemptCount      int             `json:"attempt_count"`
	MaxAttempts       int             `json:"max_attempts"`
	CurrentAttemptID  string          `json:"current_attempt_id,omitempty"`
	TerminalAttemptID string          `json:"terminal_attempt_id,omitempty"`
	NextAttemptAt     *time.Time      `json:"next_attempt_at,omitempty"`
	LastFailure       *Failure        `json:"last_failure,omitempty"`
	LastRetry         *RetryMetadata  `json:"last_retry,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	Version           uint64          `json:"version"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
	CanceledAt        *time.Time      `json:"canceled_at,omitempty"`
}

// Attempt is one immutable-history execution record. Lease tokens are never
// included in snapshots; only the monotonically increasing fence is visible.
type Attempt struct {
	ID             string          `json:"id"`
	JobID          string          `json:"job_id"`
	Number         int             `json:"number"`
	AgentID        string          `json:"agent_id"`
	State          AttemptState    `json:"state"`
	Fence          uint64          `json:"fence"`
	LeaseExpiresAt time.Time       `json:"lease_expires_at"`
	TraceID        string          `json:"trace_id,omitempty"`
	Failure        *Failure        `json:"failure,omitempty"`
	Retry          *RetryMetadata  `json:"retry,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
}

// Event is an append-only state/event stream entry. Sequence is monotonically
// increasing within a job.
type Event struct {
	ID        string          `json:"id"`
	JobID     string          `json:"job_id"`
	AttemptID string          `json:"attempt_id,omitempty"`
	Sequence  uint64          `json:"sequence"`
	Type      string          `json:"type"`
	State     State           `json:"state"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type OutboxKind string

const (
	OutboxDispatch OutboxKind = "dispatch"
	OutboxCancel   OutboxKind = "cancel"
)

// Outbox is a detached snapshot of a transactional outbox row.
type Outbox struct {
	ID              string          `json:"id"`
	Kind            OutboxKind      `json:"kind"`
	Topic           string          `json:"topic"`
	MessageID       string          `json:"message_id"`
	JobID           string          `json:"job_id"`
	Payload         json.RawMessage `json:"payload"`
	CreatedAt       time.Time       `json:"created_at"`
	PublishedAt     *time.Time      `json:"published_at,omitempty"`
	PublishAttempts int             `json:"publish_attempts"`
	LastError       string          `json:"last_error,omitempty"`
}

// OutboxClaim carries the opaque, short-lived claim needed to acknowledge a
// publish result. Token is intentionally absent from Outbox.
type OutboxClaim struct {
	Outbox    Outbox
	Token     string
	ClaimedBy string
	ExpiresAt time.Time
}

// Message is the transport-neutral message passed to a queue publisher.
type Message struct {
	ID      string
	Topic   string
	Payload []byte
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("null")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	switch err := decoder.Decode(&trailing); {
	case err == nil:
		return nil, fmt.Errorf("multiple JSON values")
	case err != io.EOF:
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func digestRequest(r Request) string {
	h := sha256.New()
	writeDigestField(h, r.projectID)
	writeDigestField(h, r.function.Name)
	writeDigestField(h, r.function.Version)
	writeDigestField(h, r.function.Digest)
	writeDigestField(h, string(r.input))
	writeDigestField(h, fmt.Sprintf("%d", r.maxAttempts))
	writeDigestField(h, r.retryPolicy.InitialBackoff.String())
	writeDigestField(h, r.retryPolicy.MaxBackoff.String())
	return hex.EncodeToString(h.Sum(nil))
}

type stringWriter interface {
	Write([]byte) (int, error)
}

func writeDigestField(w stringWriter, value string) {
	_, _ = fmt.Fprintf(w, "%d:", len(value))
	_, _ = w.Write([]byte(value))
}

func cloneBytes[T ~[]byte](value T) T {
	if value == nil {
		return nil
	}
	return append(T(nil), value...)
}

func cloneFailure(value *Failure) *Failure {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Details = cloneBytes(value.Details)
	return &cloned
}

func cloneRetry(value *RetryMetadata) *RetryMetadata {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneRequest(value Request) Request {
	value.input = cloneBytes(value.input)
	return value
}

func cloneJob(value Job) Job {
	value.Request = cloneRequest(value.Request)
	value.NextAttemptAt = cloneTime(value.NextAttemptAt)
	value.LastFailure = cloneFailure(value.LastFailure)
	value.LastRetry = cloneRetry(value.LastRetry)
	value.Result = cloneBytes(value.Result)
	value.CompletedAt = cloneTime(value.CompletedAt)
	value.CanceledAt = cloneTime(value.CanceledAt)
	return value
}

func cloneAttempt(value Attempt) Attempt {
	value.Failure = cloneFailure(value.Failure)
	value.Retry = cloneRetry(value.Retry)
	value.Result = cloneBytes(value.Result)
	value.StartedAt = cloneTime(value.StartedAt)
	value.FinishedAt = cloneTime(value.FinishedAt)
	return value
}

func cloneEvent(value Event) Event {
	value.Payload = cloneBytes(value.Payload)
	return value
}

func cloneOutbox(value Outbox) Outbox {
	value.Payload = cloneBytes(value.Payload)
	value.PublishedAt = cloneTime(value.PublishedAt)
	return value
}
