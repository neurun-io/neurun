package job

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultDispatchTopic = "jobs.execute.default"
	maxOutboxBatch       = 1000
	maxListLimit         = 200
	maxRecoveryBatch     = 1000
)

// Clock lets tests and development processes advance durable deadlines without
// sleeping. Implementations must be safe for concurrent use.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// IDGenerator supplies unpredictable aggregate IDs and opaque capability
// tokens. Implementations must be safe for concurrent use.
type IDGenerator interface {
	NewID(prefix string) (string, error)
	NewToken() (string, error)
}

type randomIDGenerator struct{}

func (randomIDGenerator) NewID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func (randomIDGenerator) NewToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

type memoryConfig struct {
	clock         Clock
	ids           IDGenerator
	dispatchTopic string
}

// MemoryOption configures the development repository.
type MemoryOption func(*memoryConfig)

func WithClock(clock Clock) MemoryOption {
	return func(config *memoryConfig) {
		if clock != nil {
			config.clock = clock
		}
	}
}

func WithIDGenerator(generator IDGenerator) MemoryOption {
	return func(config *memoryConfig) {
		if generator != nil {
			config.ids = generator
		}
	}
}

func WithDispatchTopic(topic string) MemoryOption {
	return func(config *memoryConfig) {
		if strings.TrimSpace(topic) != "" {
			config.dispatchTopic = topic
		}
	}
}

type jobRecord struct {
	job              Job
	requestDigest    string
	currentAttemptID string
	nextFence        uint64
	dispatchSequence uint64
}

type attemptRecord struct {
	attempt            Attempt
	projectID          string
	tokenHash          [sha256.Size]byte
	finalizationDigest string
}

type outboxRecord struct {
	outbox     Outbox
	projectID  string
	claimHash  [sha256.Size]byte
	claimOwner string
	claimUntil time.Time
}

// MemoryRepository is a race-safe development adapter. Every command locks the
// complete in-memory transaction, mirroring the atomicity expected from a
// PostgreSQL implementation. Returned snapshots never alias internal slices.
type MemoryRepository struct {
	mu sync.RWMutex

	clock         Clock
	ids           IDGenerator
	dispatchTopic string

	jobs        map[string]*jobRecord
	jobOrder    []string
	idempotency map[string]map[string]string
	attempts    map[string][]*attemptRecord
	attemptByID map[string]*attemptRecord
	events      map[string][]Event
	outbox      map[string]*outboxRecord
	outboxOrder []string
}

func NewMemoryRepository(options ...MemoryOption) *MemoryRepository {
	config := memoryConfig{
		clock:         systemClock{},
		ids:           randomIDGenerator{},
		dispatchTopic: defaultDispatchTopic,
	}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return &MemoryRepository{
		clock:         config.clock,
		ids:           config.ids,
		dispatchTopic: config.dispatchTopic,
		jobs:          make(map[string]*jobRecord),
		idempotency:   make(map[string]map[string]string),
		attempts:      make(map[string][]*attemptRecord),
		attemptByID:   make(map[string]*attemptRecord),
		events:        make(map[string][]Event),
		outbox:        make(map[string]*outboxRecord),
	}
}

func (repository *MemoryRepository) Accept(ctx context.Context, command AcceptCommand) (AcceptResult, error) {
	if err := ctx.Err(); err != nil {
		return AcceptResult{}, err
	}
	request, err := validateImmutableRequest(command.Request)
	if err != nil {
		return AcceptResult{}, err
	}
	if len(command.IdempotencyKey) > 256 {
		return AcceptResult{}, fmt.Errorf("%w: idempotency key exceeds 256 bytes", ErrInvalid)
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return AcceptResult{}, err
	}

	if command.IdempotencyKey != "" {
		projectKeys := repository.idempotency[request.ProjectID()]
		if existingID := projectKeys[command.IdempotencyKey]; existingID != "" {
			existing := repository.jobs[existingID]
			if existing.requestDigest != request.Digest() {
				return AcceptResult{}, ErrIdempotencyConflict
			}
			return AcceptResult{Job: cloneJob(existing.job), Duplicate: true}, nil
		}
	}

	jobID, err := repository.ids.NewID("job")
	if err != nil {
		return AcceptResult{}, fmt.Errorf("generate job id: %w", err)
	}
	if _, exists := repository.jobs[jobID]; exists {
		return AcceptResult{}, fmt.Errorf("%w: generated duplicate job id", ErrInvalid)
	}
	now := repository.now()
	record := &jobRecord{
		job: Job{
			ID:             jobID,
			ProjectID:      request.ProjectID(),
			IdempotencyKey: command.IdempotencyKey,
			Request:        cloneRequest(request),
			State:          StateAccepted,
			MaxAttempts:    request.MaxAttempts(),
			Version:        1,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		requestDigest: request.Digest(),
	}

	repository.jobs[jobID] = record
	repository.jobOrder = append(repository.jobOrder, jobID)
	if command.IdempotencyKey != "" {
		projectKeys := repository.idempotency[request.ProjectID()]
		if projectKeys == nil {
			projectKeys = make(map[string]string)
			repository.idempotency[request.ProjectID()] = projectKeys
		}
		projectKeys[command.IdempotencyKey] = jobID
	}
	repository.appendEventLocked(record, "", "job.accepted", nil, now)
	outbox := repository.enqueueDispatchLocked(record, now)
	repository.transitionLocked(record, StateQueued, now)
	repository.appendEventLocked(record, "", "job.queued", mustJSON(struct {
		MessageID string `json:"message_id"`
	}{MessageID: outbox.outbox.MessageID}), now)

	return AcceptResult{Job: cloneJob(record.job)}, nil
}

func (repository *MemoryRepository) Get(ctx context.Context, projectID, jobID string) (Job, error) {
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	record, err := repository.projectJobLocked(projectID, jobID)
	if err != nil {
		return Job{}, err
	}
	return cloneJob(record.job), nil
}

func (repository *MemoryRepository) List(ctx context.Context, projectID string, options ListOptions) (JobPage, error) {
	if err := ctx.Err(); err != nil {
		return JobPage{}, err
	}
	if strings.TrimSpace(projectID) == "" {
		return JobPage{}, fmt.Errorf("%w: project id is required", ErrInvalid)
	}
	limit := options.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > maxListLimit {
		return JobPage{}, fmt.Errorf("%w: list limit must be between 1 and %d", ErrInvalid, maxListLimit)
	}
	stateFilter := make(map[State]struct{}, len(options.States))
	for _, state := range options.States {
		if !state.Valid() {
			return JobPage{}, fmt.Errorf("%w: unknown state %q", ErrInvalid, state)
		}
		stateFilter[state] = struct{}{}
	}
	cursor, err := decodeListCursor(options.Cursor)
	if err != nil {
		return JobPage{}, err
	}

	repository.mu.RLock()
	defer repository.mu.RUnlock()

	matches := make([]Job, 0)
	for _, jobID := range repository.jobOrder {
		record := repository.jobs[jobID]
		if record.job.ProjectID != projectID {
			continue
		}
		if len(stateFilter) > 0 {
			if _, ok := stateFilter[record.job.State]; !ok {
				continue
			}
		}
		if !options.CreatedAfter.IsZero() && !record.job.CreatedAt.After(options.CreatedAfter) {
			continue
		}
		if cursor.ID != "" && !comesAfterCursor(record.job, cursor) {
			continue
		}
		matches = append(matches, cloneJob(record.job))
	}
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].CreatedAt.Equal(matches[right].CreatedAt) {
			return matches[left].ID > matches[right].ID
		}
		return matches[left].CreatedAt.After(matches[right].CreatedAt)
	})

	page := JobPage{}
	if len(matches) <= limit {
		page.Jobs = matches
		return page, nil
	}
	page.Jobs = matches[:limit]
	last := page.Jobs[len(page.Jobs)-1]
	page.NextCursor = encodeListCursor(listCursor{
		CreatedAt: last.CreatedAt.UnixNano(),
		ID:        last.ID,
	})
	return page, nil
}

func (repository *MemoryRepository) Events(ctx context.Context, projectID, jobID string) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	if _, err := repository.projectJobLocked(projectID, jobID); err != nil {
		return nil, err
	}
	stored := repository.events[jobID]
	result := make([]Event, len(stored))
	for index := range stored {
		result[index] = cloneEvent(stored[index])
	}
	return result, nil
}

func (repository *MemoryRepository) Attempts(ctx context.Context, projectID, jobID string) ([]Attempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	if _, err := repository.projectJobLocked(projectID, jobID); err != nil {
		return nil, err
	}
	stored := repository.attempts[jobID]
	result := make([]Attempt, len(stored))
	for index := range stored {
		result[index] = cloneAttempt(stored[index].attempt)
	}
	return result, nil
}

func (repository *MemoryRepository) ClaimOutbox(ctx context.Context, command ClaimOutboxCommand) ([]OutboxClaim, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(command.Owner) == "" {
		return nil, fmt.Errorf("%w: outbox claim owner is required", ErrInvalid)
	}
	limit := command.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > maxOutboxBatch {
		return nil, fmt.Errorf("%w: outbox claim limit must be between 1 and %d", ErrInvalid, maxOutboxBatch)
	}
	if command.TTL <= 0 {
		return nil, fmt.Errorf("%w: outbox claim ttl must be positive", ErrInvalid)
	}

	// Generate all possible claim tokens before entering the transaction so a
	// random-source failure can never leave a partially returned claim batch.
	tokens := make([]string, limit)
	for index := range tokens {
		token, err := repository.ids.NewToken()
		if err != nil {
			return nil, fmt.Errorf("generate outbox claim token: %w", err)
		}
		tokens[index] = token
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := repository.now()
	claims := make([]OutboxClaim, 0, limit)
	for _, outboxID := range repository.outboxOrder {
		if len(claims) == limit {
			break
		}
		record := repository.outbox[outboxID]
		if record.outbox.PublishedAt != nil {
			continue
		}
		if record.claimOwner != "" && record.claimUntil.After(now) {
			continue
		}
		token := tokens[len(claims)]
		record.claimHash = sha256.Sum256([]byte(token))
		record.claimOwner = command.Owner
		record.claimUntil = now.Add(command.TTL)
		record.outbox.PublishAttempts++
		claims = append(claims, OutboxClaim{
			Outbox:    cloneOutbox(record.outbox),
			Token:     token,
			ClaimedBy: command.Owner,
			ExpiresAt: record.claimUntil,
		})
	}
	return claims, nil
}

func (repository *MemoryRepository) MarkOutboxPublished(ctx context.Context, outboxID, token string) (Outbox, error) {
	if err := ctx.Err(); err != nil {
		return Outbox{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Outbox{}, err
	}
	record := repository.outbox[outboxID]
	if record == nil {
		return Outbox{}, ErrNotFound
	}
	if record.outbox.PublishedAt != nil {
		return cloneOutbox(record.outbox), nil
	}
	now := repository.now()
	if !repository.validOutboxClaimLocked(record, token, now) {
		return Outbox{}, ErrOutboxClaimLost
	}

	record.outbox.PublishedAt = cloneTime(&now)
	record.outbox.LastError = ""
	repository.clearOutboxClaimLocked(record)
	return cloneOutbox(record.outbox), nil
}

func (repository *MemoryRepository) MarkOutboxFailed(ctx context.Context, outboxID, token, failure string) (Outbox, error) {
	if err := ctx.Err(); err != nil {
		return Outbox{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Outbox{}, err
	}
	record := repository.outbox[outboxID]
	if record == nil {
		return Outbox{}, ErrNotFound
	}
	if record.outbox.PublishedAt != nil {
		return cloneOutbox(record.outbox), nil
	}
	now := repository.now()
	if !repository.validOutboxClaimLocked(record, token, now) {
		return Outbox{}, ErrOutboxClaimLost
	}
	if len(failure) > 2048 {
		failure = failure[:2048]
	}
	record.outbox.LastError = failure
	repository.clearOutboxClaimLocked(record)
	return cloneOutbox(record.outbox), nil
}

func (repository *MemoryRepository) AcquireLease(ctx context.Context, command LeaseCommand) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return Lease{}, err
	}
	if strings.TrimSpace(command.ProjectID) == "" || strings.TrimSpace(command.JobID) == "" {
		return Lease{}, fmt.Errorf("%w: project id and job id are required", ErrInvalid)
	}
	if strings.TrimSpace(command.AgentID) == "" {
		return Lease{}, fmt.Errorf("%w: agent id is required", ErrInvalid)
	}
	if command.TTL <= 0 {
		return Lease{}, fmt.Errorf("%w: lease ttl must be positive", ErrInvalid)
	}
	attemptID, err := repository.ids.NewID("att")
	if err != nil {
		return Lease{}, fmt.Errorf("generate attempt id: %w", err)
	}
	token, err := repository.ids.NewToken()
	if err != nil {
		return Lease{}, fmt.Errorf("generate lease token: %w", err)
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Lease{}, err
	}
	record, err := repository.projectJobLocked(command.ProjectID, command.JobID)
	if err != nil {
		return Lease{}, err
	}
	if record.job.State != StateQueued {
		return Lease{}, fmt.Errorf("%w: current state is %s", ErrNotClaimable, record.job.State)
	}
	if record.job.AttemptCount >= record.job.MaxAttempts {
		return Lease{}, ErrAttemptsExhausted
	}
	if _, exists := repository.attemptByID[attemptID]; exists {
		return Lease{}, fmt.Errorf("%w: generated duplicate attempt id", ErrInvalid)
	}

	now := repository.now()
	expiry := now.Add(command.TTL)
	fence := record.nextFence + 1
	attemptRecord := &attemptRecord{
		attempt: Attempt{
			ID:             attemptID,
			JobID:          record.job.ID,
			Number:         record.job.AttemptCount + 1,
			AgentID:        command.AgentID,
			State:          AttemptLeased,
			Fence:          fence,
			LeaseExpiresAt: expiry,
			TraceID:        command.TraceID,
			CreatedAt:      now,
		},
		projectID: command.ProjectID,
		tokenHash: sha256.Sum256([]byte(token)),
	}
	repository.attempts[record.job.ID] = append(repository.attempts[record.job.ID], attemptRecord)
	repository.attemptByID[attemptID] = attemptRecord
	record.nextFence = fence
	record.currentAttemptID = attemptID
	record.job.CurrentAttemptID = attemptID
	record.job.AttemptCount++
	repository.transitionLocked(record, StateLeased, now)
	repository.appendEventLocked(record, attemptID, "job.leased", mustJSON(struct {
		AgentID   string    `json:"agent_id"`
		Fence     uint64    `json:"fence"`
		ExpiresAt time.Time `json:"expires_at"`
	}{
		AgentID: command.AgentID, Fence: fence, ExpiresAt: expiry,
	}), now)

	return Lease{
		Job:       cloneJob(record.job),
		Attempt:   cloneAttempt(attemptRecord.attempt),
		Token:     token,
		Fence:     fence,
		ExpiresAt: expiry,
	}, nil
}

func (repository *MemoryRepository) Start(ctx context.Context, lease LeaseRef) (Attempt, error) {
	if err := ctx.Err(); err != nil {
		return Attempt{}, err
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Attempt{}, err
	}
	now := repository.now()
	record, attempt, err := repository.validateLeaseLocked(lease, now)
	if err != nil {
		return Attempt{}, err
	}
	if record.job.State != StateLeased || attempt.attempt.State != AttemptLeased {
		return Attempt{}, fmt.Errorf("%w: lease cannot start from job %s attempt %s", ErrInvalidTransition, record.job.State, attempt.attempt.State)
	}

	attempt.attempt.State = AttemptRunning
	attempt.attempt.StartedAt = cloneTime(&now)
	repository.transitionLocked(record, StateRunning, now)
	repository.appendEventLocked(record, attempt.attempt.ID, "job.started", nil, now)
	return cloneAttempt(attempt.attempt), nil
}

func (repository *MemoryRepository) RenewLease(ctx context.Context, command RenewLeaseCommand) (Lease, error) {
	if err := ctx.Err(); err != nil {
		return Lease{}, err
	}
	if command.TTL <= 0 {
		return Lease{}, fmt.Errorf("%w: renewal ttl must be positive", ErrInvalid)
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Lease{}, err
	}
	now := repository.now()
	record, attempt, err := repository.validateLeaseLocked(command.Lease, now)
	if err != nil {
		return Lease{}, err
	}
	if record.job.State != StateLeased && record.job.State != StateRunning {
		return Lease{}, ErrLeaseLost
	}
	if attempt.attempt.State != AttemptLeased && attempt.attempt.State != AttemptRunning {
		return Lease{}, ErrLeaseLost
	}

	expiry := now.Add(command.TTL)
	attempt.attempt.LeaseExpiresAt = expiry
	record.job.UpdatedAt = now
	record.job.Version++
	repository.appendEventLocked(record, attempt.attempt.ID, "job.lease_renewed", mustJSON(struct {
		Fence     uint64    `json:"fence"`
		ExpiresAt time.Time `json:"expires_at"`
	}{Fence: attempt.attempt.Fence, ExpiresAt: expiry}), now)
	return Lease{
		Job:       cloneJob(record.job),
		Attempt:   cloneAttempt(attempt.attempt),
		Token:     command.Lease.Token,
		Fence:     attempt.attempt.Fence,
		ExpiresAt: expiry,
	}, nil
}

func (repository *MemoryRepository) Finalize(ctx context.Context, command FinalizeCommand) (FinalizeResult, error) {
	if err := ctx.Err(); err != nil {
		return FinalizeResult{}, err
	}
	normalized, digest, err := normalizeFinalization(command)
	if err != nil {
		return FinalizeResult{}, err
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return FinalizeResult{}, err
	}
	record, err := repository.projectJobLocked(normalized.Lease.ProjectID, normalized.Lease.JobID)
	if err != nil {
		return FinalizeResult{}, err
	}
	attempt := repository.attemptByID[normalized.Lease.AttemptID]
	if attempt == nil || attempt.projectID != normalized.Lease.ProjectID || attempt.attempt.JobID != record.job.ID {
		return FinalizeResult{}, ErrLeaseLost
	}
	if !tokenMatches(attempt.tokenHash, normalized.Lease.Token) || attempt.attempt.Fence != normalized.Lease.Fence {
		return FinalizeResult{}, ErrLeaseLost
	}
	if attempt.finalizationDigest != "" {
		if attempt.finalizationDigest != digest {
			return FinalizeResult{}, ErrFinalizationConflict
		}
		return FinalizeResult{
			Job:       cloneJob(record.job),
			Attempt:   cloneAttempt(attempt.attempt),
			Duplicate: true,
		}, nil
	}

	now := repository.now()
	if record.currentAttemptID != attempt.attempt.ID ||
		record.job.CurrentAttemptID != attempt.attempt.ID ||
		record.job.State != StateRunning ||
		attempt.attempt.State != AttemptRunning ||
		!attempt.attempt.LeaseExpiresAt.After(now) {
		return FinalizeResult{}, ErrLeaseLost
	}

	attempt.finalizationDigest = digest
	attempt.attempt.State = attemptStateForOutcome(normalized.Outcome)
	attempt.attempt.Result = cloneBytes(normalized.Result)
	attempt.attempt.Failure = cloneFailure(normalized.Failure)
	attempt.attempt.FinishedAt = cloneTime(&now)
	record.currentAttemptID = ""
	record.job.CurrentAttemptID = ""
	record.job.Result = cloneBytes(normalized.Result)
	record.job.LastFailure = cloneFailure(normalized.Failure)
	record.job.NextAttemptAt = nil

	if normalized.Retry != nil {
		retry := &RetryMetadata{
			Reason:          normalized.Retry.Reason,
			AttemptNumber:   attempt.attempt.Number,
			MaxAttempts:     record.job.MaxAttempts,
			Backoff:         normalized.Retry.After,
			FailureCategory: normalized.Failure.Category,
		}
		if attempt.attempt.Number < record.job.MaxAttempts {
			retry.ScheduledAt = now.Add(normalized.Retry.After)
			attempt.attempt.Retry = cloneRetry(retry)
			record.job.LastRetry = cloneRetry(retry)
			record.job.NextAttemptAt = cloneTime(&retry.ScheduledAt)
			repository.transitionLocked(record, StateRetryWait, now)
			repository.appendEventLocked(record, attempt.attempt.ID, "job.retry_wait", mustJSON(retry), now)
		} else {
			retry.ScheduledAt = now
			retry.Exhausted = true
			attempt.attempt.Retry = cloneRetry(retry)
			record.job.LastRetry = cloneRetry(retry)
			record.job.TerminalAttemptID = attempt.attempt.ID
			record.job.CompletedAt = cloneTime(&now)
			repository.transitionLocked(record, StateDeadLettered, now)
			repository.appendEventLocked(record, attempt.attempt.ID, "job.dead_lettered", mustJSON(retry), now)
		}
	} else {
		record.job.TerminalAttemptID = attempt.attempt.ID
		record.job.CompletedAt = cloneTime(&now)
		if normalized.Outcome == StateCanceled {
			record.job.CanceledAt = cloneTime(&now)
		}
		repository.transitionLocked(record, normalized.Outcome, now)
		repository.appendEventLocked(record, attempt.attempt.ID, eventTypeForState(normalized.Outcome), failurePayload(normalized.Failure), now)
	}

	return FinalizeResult{
		Job:     cloneJob(record.job),
		Attempt: cloneAttempt(attempt.attempt),
	}, nil
}

func (repository *MemoryRepository) Cancel(ctx context.Context, projectID, jobID, reason string) (CancelResult, error) {
	if err := ctx.Err(); err != nil {
		return CancelResult{}, err
	}
	if strings.TrimSpace(reason) == "" {
		reason = "requested"
	}
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return CancelResult{}, err
	}
	record, err := repository.projectJobLocked(projectID, jobID)
	if err != nil {
		return CancelResult{}, err
	}
	if record.job.State.Terminal() {
		return CancelResult{Job: cloneJob(record.job), Duplicate: true}, nil
	}
	if !CanTransition(record.job.State, StateCanceled) {
		return CancelResult{}, fmt.Errorf("%w: cannot cancel %s", ErrInvalidTransition, record.job.State)
	}

	now := repository.now()
	result := CancelResult{}
	var attemptID string
	if record.currentAttemptID != "" {
		attempt := repository.attemptByID[record.currentAttemptID]
		if attempt != nil && !attempt.attempt.State.Terminal() {
			attemptID = attempt.attempt.ID
			record.job.TerminalAttemptID = attemptID
			result.AgentID = attempt.attempt.AgentID
			result.WasRunning = attempt.attempt.State == AttemptRunning
			attempt.attempt.State = AttemptCanceled
			attempt.attempt.Failure = &Failure{
				Category: "canceled",
				Message:  reason,
			}
			attempt.attempt.FinishedAt = cloneTime(&now)
			cancelOutbox := repository.enqueueCancellationLocked(record, attempt, reason, now)
			outbox := cloneOutbox(cancelOutbox.outbox)
			result.CancellationMsg = &outbox
		}
	}
	record.currentAttemptID = ""
	record.job.CurrentAttemptID = ""
	record.job.CanceledAt = cloneTime(&now)
	record.job.CompletedAt = cloneTime(&now)
	record.job.LastFailure = &Failure{Category: "canceled", Message: reason}
	repository.transitionLocked(record, StateCanceled, now)
	repository.appendEventLocked(record, attemptID, "job.canceled", mustJSON(struct {
		Reason string `json:"reason"`
	}{Reason: reason}), now)
	result.Job = cloneJob(record.job)
	return result, nil
}

func (repository *MemoryRepository) EnqueueDueRetries(ctx context.Context, limit int) ([]Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > maxRecoveryBatch {
		return nil, fmt.Errorf("%w: retry enqueue limit must be between 1 and %d", ErrInvalid, maxRecoveryBatch)
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := repository.now()
	candidates := make([]*jobRecord, 0)
	for _, record := range repository.jobs {
		if record.job.State == StateRetryWait &&
			record.job.NextAttemptAt != nil &&
			!record.job.NextAttemptAt.After(now) {
			candidates = append(candidates, record)
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftAt := candidates[left].job.NextAttemptAt
		rightAt := candidates[right].job.NextAttemptAt
		if leftAt.Equal(*rightAt) {
			return candidates[left].job.ID < candidates[right].job.ID
		}
		return leftAt.Before(*rightAt)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	result := make([]Job, 0, len(candidates))
	for _, record := range candidates {
		record.job.NextAttemptAt = nil
		repository.transitionLocked(record, StateQueued, now)
		outbox := repository.enqueueDispatchLocked(record, now)
		repository.appendEventLocked(record, "", "job.queued", mustJSON(struct {
			Reason    string `json:"reason"`
			MessageID string `json:"message_id"`
		}{Reason: "retry_due", MessageID: outbox.outbox.MessageID}), now)
		result = append(result, cloneJob(record.job))
	}
	return result, nil
}

func (repository *MemoryRepository) RecoverExpiredLeases(ctx context.Context, limit int) ([]Recovery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > maxRecoveryBatch {
		return nil, fmt.Errorf("%w: recovery limit must be between 1 and %d", ErrInvalid, maxRecoveryBatch)
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := repository.now()
	type candidate struct {
		job     *jobRecord
		attempt *attemptRecord
	}
	candidates := make([]candidate, 0)
	for _, record := range repository.jobs {
		if record.job.State != StateLeased && record.job.State != StateRunning {
			continue
		}
		attempt := repository.attemptByID[record.currentAttemptID]
		if attempt != nil && !attempt.attempt.LeaseExpiresAt.After(now) {
			candidates = append(candidates, candidate{job: record, attempt: attempt})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftAttempt := candidates[left].attempt.attempt
		rightAttempt := candidates[right].attempt.attempt
		if leftAttempt.LeaseExpiresAt.Equal(rightAttempt.LeaseExpiresAt) {
			return leftAttempt.JobID < rightAttempt.JobID
		}
		return leftAttempt.LeaseExpiresAt.Before(rightAttempt.LeaseExpiresAt)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	recoveries := make([]Recovery, 0, len(candidates))
	for _, candidate := range candidates {
		record := candidate.job
		attempt := candidate.attempt
		wasRunning := attempt.attempt.State == AttemptRunning
		category := "lease_expired"
		eventType := "attempt.lease_expired"
		if wasRunning {
			category = "agent_lost"
			eventType = "attempt.failed"
			attempt.attempt.State = AttemptFailed
		} else {
			attempt.attempt.State = AttemptLeaseExpired
		}
		failure := &Failure{
			Category:  category,
			Message:   "execution lease expired",
			Retryable: true,
		}
		attempt.attempt.Failure = cloneFailure(failure)
		attempt.attempt.FinishedAt = cloneTime(&now)
		record.currentAttemptID = ""
		record.job.CurrentAttemptID = ""
		record.job.LastFailure = cloneFailure(failure)
		repository.appendEventLocked(record, attempt.attempt.ID, eventType, failurePayload(failure), now)

		recovery := Recovery{}
		if attempt.attempt.Number < record.job.MaxAttempts {
			retry := &RetryMetadata{
				Reason:          category,
				AttemptNumber:   attempt.attempt.Number,
				MaxAttempts:     record.job.MaxAttempts,
				ScheduledAt:     now,
				FailureCategory: category,
			}
			attempt.attempt.Retry = cloneRetry(retry)
			record.job.LastRetry = cloneRetry(retry)
			repository.transitionLocked(record, StateQueued, now)
			outbox := repository.enqueueDispatchLocked(record, now)
			repository.appendEventLocked(record, attempt.attempt.ID, "job.queued", mustJSON(struct {
				Reason    string `json:"reason"`
				MessageID string `json:"message_id"`
			}{Reason: category, MessageID: outbox.outbox.MessageID}), now)
			recovery.Requeued = true
		} else {
			retry := &RetryMetadata{
				Reason:          category,
				AttemptNumber:   attempt.attempt.Number,
				MaxAttempts:     record.job.MaxAttempts,
				ScheduledAt:     now,
				FailureCategory: category,
				Exhausted:       true,
			}
			attempt.attempt.Retry = cloneRetry(retry)
			record.job.LastRetry = cloneRetry(retry)
			record.job.TerminalAttemptID = attempt.attempt.ID
			record.job.CompletedAt = cloneTime(&now)
			repository.transitionLocked(record, StateDeadLettered, now)
			repository.appendEventLocked(record, attempt.attempt.ID, "job.dead_lettered", mustJSON(retry), now)
			recovery.Exhausted = true
		}
		recovery.Job = cloneJob(record.job)
		recovery.Attempt = cloneAttempt(attempt.attempt)
		recoveries = append(recoveries, recovery)
	}
	return recoveries, nil
}

// OutboxRecords returns all outbox rows for development diagnostics. It is not
// part of Repository because production callers should use claims.
func (repository *MemoryRepository) OutboxRecords(ctx context.Context) ([]Outbox, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repository.mu.RLock()
	defer repository.mu.RUnlock()
	result := make([]Outbox, 0, len(repository.outboxOrder))
	for _, outboxID := range repository.outboxOrder {
		result = append(result, cloneOutbox(repository.outbox[outboxID].outbox))
	}
	return result, nil
}

func (repository *MemoryRepository) now() time.Time {
	return repository.clock.Now().UTC()
}

func (repository *MemoryRepository) projectJobLocked(projectID, jobID string) (*jobRecord, error) {
	record := repository.jobs[jobID]
	if record == nil || record.job.ProjectID != projectID {
		return nil, ErrNotFound
	}
	return record, nil
}

func (repository *MemoryRepository) transitionLocked(record *jobRecord, state State, now time.Time) {
	if !CanTransition(record.job.State, state) {
		panic(fmt.Sprintf("job: internal invalid transition %s -> %s", record.job.State, state))
	}
	record.job.State = state
	record.job.UpdatedAt = now
	record.job.Version++
}

func (repository *MemoryRepository) appendEventLocked(record *jobRecord, attemptID, eventType string, payload json.RawMessage, now time.Time) {
	sequence := uint64(len(repository.events[record.job.ID]) + 1)
	event := Event{
		ID:        deterministicID("evt", record.job.ID, fmt.Sprintf("%d", sequence)),
		JobID:     record.job.ID,
		AttemptID: attemptID,
		Sequence:  sequence,
		Type:      eventType,
		State:     record.job.State,
		Payload:   cloneBytes(payload),
		CreatedAt: now,
	}
	repository.events[record.job.ID] = append(repository.events[record.job.ID], event)
}

func (repository *MemoryRepository) enqueueDispatchLocked(record *jobRecord, now time.Time) *outboxRecord {
	record.dispatchSequence++
	messageID := deterministicID(
		"msg",
		"dispatch",
		record.job.ProjectID,
		record.job.ID,
		record.requestDigest,
		fmt.Sprintf("%d", record.dispatchSequence),
	)
	payload := mustJSON(struct {
		MessageID      string      `json:"message_id"`
		JobID          string      `json:"job_id"`
		ProjectID      string      `json:"project_id"`
		RequestDigest  string      `json:"request_digest"`
		DispatchNumber uint64      `json:"dispatch_number"`
		Function       FunctionRef `json:"function"`
	}{
		MessageID:      messageID,
		JobID:          record.job.ID,
		ProjectID:      record.job.ProjectID,
		RequestDigest:  record.requestDigest,
		DispatchNumber: record.dispatchSequence,
		Function:       record.job.Request.Function(),
	})
	outbox := &outboxRecord{
		outbox: Outbox{
			ID:        deterministicID("obx", messageID),
			Kind:      OutboxDispatch,
			Topic:     repository.dispatchTopic,
			MessageID: messageID,
			JobID:     record.job.ID,
			Payload:   payload,
			CreatedAt: now,
		},
		projectID: record.job.ProjectID,
	}
	repository.insertOutboxLocked(outbox)
	return outbox
}

func (repository *MemoryRepository) enqueueCancellationLocked(record *jobRecord, attempt *attemptRecord, reason string, now time.Time) *outboxRecord {
	messageID := deterministicID(
		"msg",
		"cancel",
		record.job.ProjectID,
		record.job.ID,
		attempt.attempt.ID,
		fmt.Sprintf("%d", attempt.attempt.Fence),
	)
	payload := mustJSON(struct {
		MessageID string `json:"message_id"`
		JobID     string `json:"job_id"`
		ProjectID string `json:"project_id"`
		AttemptID string `json:"attempt_id"`
		Fence     uint64 `json:"fence"`
		Reason    string `json:"reason"`
	}{
		MessageID: messageID,
		JobID:     record.job.ID,
		ProjectID: record.job.ProjectID,
		AttemptID: attempt.attempt.ID,
		Fence:     attempt.attempt.Fence,
		Reason:    reason,
	})
	outbox := &outboxRecord{
		outbox: Outbox{
			ID:        deterministicID("obx", messageID),
			Kind:      OutboxCancel,
			Topic:     "jobs.cancel." + record.job.ID,
			MessageID: messageID,
			JobID:     record.job.ID,
			Payload:   payload,
			CreatedAt: now,
		},
		projectID: record.job.ProjectID,
	}
	repository.insertOutboxLocked(outbox)
	return outbox
}

func (repository *MemoryRepository) insertOutboxLocked(record *outboxRecord) {
	if existing := repository.outbox[record.outbox.ID]; existing != nil {
		if existing.outbox.MessageID != record.outbox.MessageID ||
			existing.outbox.Topic != record.outbox.Topic ||
			!jsonEqual(existing.outbox.Payload, record.outbox.Payload) {
			panic("job: deterministic outbox id collision")
		}
		return
	}
	repository.outbox[record.outbox.ID] = record
	repository.outboxOrder = append(repository.outboxOrder, record.outbox.ID)
}

func (repository *MemoryRepository) validateLeaseLocked(reference LeaseRef, now time.Time) (*jobRecord, *attemptRecord, error) {
	record, err := repository.projectJobLocked(reference.ProjectID, reference.JobID)
	if err != nil {
		return nil, nil, err
	}
	attempt := repository.attemptByID[reference.AttemptID]
	if attempt == nil ||
		attempt.projectID != reference.ProjectID ||
		attempt.attempt.JobID != reference.JobID ||
		record.currentAttemptID != reference.AttemptID ||
		record.job.CurrentAttemptID != reference.AttemptID ||
		attempt.attempt.Fence != reference.Fence ||
		!tokenMatches(attempt.tokenHash, reference.Token) ||
		!attempt.attempt.LeaseExpiresAt.After(now) {
		return nil, nil, ErrLeaseLost
	}
	return record, attempt, nil
}

func (repository *MemoryRepository) validOutboxClaimLocked(record *outboxRecord, token string, now time.Time) bool {
	return record.claimOwner != "" &&
		record.claimUntil.After(now) &&
		tokenMatches(record.claimHash, token)
}

func (repository *MemoryRepository) clearOutboxClaimLocked(record *outboxRecord) {
	record.claimHash = [sha256.Size]byte{}
	record.claimOwner = ""
	record.claimUntil = time.Time{}
}

func validateImmutableRequest(request Request) (Request, error) {
	if request.projectID == "" ||
		request.function.Name == "" ||
		request.function.Version == "" ||
		request.function.Digest == "" ||
		request.maxAttempts < 1 ||
		request.maxAttempts > MaxAttempts ||
		request.digest == "" {
		return Request{}, fmt.Errorf("%w: request must be created with NewRequest", ErrInvalid)
	}
	if request.digest != digestRequest(request) {
		return Request{}, fmt.Errorf("%w: immutable request digest mismatch", ErrInvalid)
	}
	return cloneRequest(request), nil
}

func normalizeFinalization(command FinalizeCommand) (FinalizeCommand, string, error) {
	switch command.Outcome {
	case StateSucceeded, StateRejected, StateFailed, StateCanceled:
	default:
		return FinalizeCommand{}, "", fmt.Errorf("%w: unsupported final outcome %q", ErrInvalid, command.Outcome)
	}
	if strings.TrimSpace(command.Lease.ProjectID) == "" ||
		strings.TrimSpace(command.Lease.JobID) == "" ||
		strings.TrimSpace(command.Lease.AttemptID) == "" ||
		command.Lease.Token == "" ||
		command.Lease.Fence == 0 {
		return FinalizeCommand{}, "", fmt.Errorf("%w: complete lease reference is required", ErrInvalid)
	}

	normalized := command
	if len(command.Result) > 0 {
		result, err := canonicalJSON(command.Result)
		if err != nil {
			return FinalizeCommand{}, "", fmt.Errorf("%w: result: %v", ErrInvalid, err)
		}
		normalized.Result = result
	}
	normalized.Failure = cloneFailure(command.Failure)
	if normalized.Failure != nil && len(normalized.Failure.Details) > 0 {
		details, err := canonicalJSON(normalized.Failure.Details)
		if err != nil {
			return FinalizeCommand{}, "", fmt.Errorf("%w: failure details: %v", ErrInvalid, err)
		}
		normalized.Failure.Details = details
	}

	switch normalized.Outcome {
	case StateSucceeded:
		if normalized.Failure != nil {
			return FinalizeCommand{}, "", fmt.Errorf("%w: succeeded outcome cannot include failure metadata", ErrInvalid)
		}
	case StateRejected, StateFailed:
		if normalized.Failure == nil || strings.TrimSpace(normalized.Failure.Category) == "" {
			return FinalizeCommand{}, "", fmt.Errorf("%w: failed and rejected outcomes require a failure category", ErrInvalid)
		}
	}
	if normalized.Retry != nil {
		if normalized.Outcome != StateFailed && normalized.Outcome != StateRejected {
			return FinalizeCommand{}, "", fmt.Errorf("%w: only failed or rejected attempts can retry", ErrInvalid)
		}
		if normalized.Retry.After < 0 {
			return FinalizeCommand{}, "", fmt.Errorf("%w: retry delay cannot be negative", ErrInvalid)
		}
		if !normalized.Failure.Retryable {
			return FinalizeCommand{}, "", fmt.Errorf("%w: failure is not retryable", ErrInvalid)
		}
		retry := *normalized.Retry
		if strings.TrimSpace(retry.Reason) == "" {
			retry.Reason = normalized.Failure.Category
		}
		normalized.Retry = &retry
	}

	type finalizationDigest struct {
		AttemptID string          `json:"attempt_id"`
		Fence     uint64          `json:"fence"`
		Outcome   State           `json:"outcome"`
		Result    json.RawMessage `json:"result,omitempty"`
		Failure   *Failure        `json:"failure,omitempty"`
		Retry     *RetryDirective `json:"retry,omitempty"`
	}
	encoded := mustJSON(finalizationDigest{
		AttemptID: normalized.Lease.AttemptID,
		Fence:     normalized.Lease.Fence,
		Outcome:   normalized.Outcome,
		Result:    normalized.Result,
		Failure:   normalized.Failure,
		Retry:     normalized.Retry,
	})
	sum := sha256.Sum256(encoded)
	return normalized, hex.EncodeToString(sum[:]), nil
}

func attemptStateForOutcome(outcome State) AttemptState {
	switch outcome {
	case StateSucceeded:
		return AttemptSucceeded
	case StateRejected:
		return AttemptRejected
	case StateFailed:
		return AttemptFailed
	case StateCanceled:
		return AttemptCanceled
	default:
		panic("job: invalid attempt outcome")
	}
}

func eventTypeForState(state State) string {
	return "job." + string(state)
}

func failurePayload(failure *Failure) json.RawMessage {
	if failure == nil {
		return nil
	}
	return mustJSON(failure)
}

func tokenMatches(expected [sha256.Size]byte, token string) bool {
	actual := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(expected[:], actual[:]) == 1
}

func deterministicID(prefix string, values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		writeDigestField(hash, value)
	}
	return prefix + "_" + hex.EncodeToString(hash.Sum(nil))
}

func mustJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func jsonEqual(left, right []byte) bool {
	return string(left) == string(right)
}

type listCursor struct {
	CreatedAt int64  `json:"created_at"`
	ID        string `json:"id"`
}

func encodeListCursor(cursor listCursor) string {
	return base64.RawURLEncoding.EncodeToString(mustJSON(cursor))
}

func decodeListCursor(encoded string) (listCursor, error) {
	if encoded == "" {
		return listCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return listCursor{}, ErrInvalidCursor
	}
	var cursor listCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.ID == "" {
		return listCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

func comesAfterCursor(value Job, cursor listCursor) bool {
	createdAt := value.CreatedAt.UnixNano()
	return createdAt < cursor.CreatedAt ||
		(createdAt == cursor.CreatedAt && value.ID < cursor.ID)
}

// Compile-time contract check.
var _ Repository = (*MemoryRepository)(nil)
