package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type manualClock struct {
	mu  sync.RWMutex
	now time.Time
}

func newManualClock() *manualClock {
	return &manualClock{now: time.Date(2026, 7, 28, 2, 26, 0, 0, time.UTC)}
}

func (clock *manualClock) Now() time.Time {
	clock.mu.RLock()
	defer clock.mu.RUnlock()
	return clock.now
}

func (clock *manualClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(duration)
}

type sequentialIDs struct {
	value atomic.Uint64
}

func (ids *sequentialIDs) NewID(prefix string) (string, error) {
	return fmt.Sprintf("%s_%08d", prefix, ids.value.Add(1)), nil
}

func (ids *sequentialIDs) NewToken() (string, error) {
	return fmt.Sprintf("opaque_%08d", ids.value.Add(1)), nil
}

func newTestRepository() (*MemoryRepository, *manualClock) {
	clock := newManualClock()
	return NewMemoryRepository(WithClock(clock), WithIDGenerator(&sequentialIDs{})), clock
}

func testRequest(t *testing.T, projectID string, input string, maxAttempts int) Request {
	t.Helper()
	request, err := NewRequest(
		projectID,
		FunctionRef{
			Name:    "http.fetch",
			Version: "1.2.3",
			Digest:  "sha256:abc123",
		},
		json.RawMessage(input),
		RequestOptions{
			MaxAttempts: maxAttempts,
			RetryPolicy: RetryPolicy{
				InitialBackoff: time.Second,
				MaxBackoff:     time.Minute,
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return request
}

func acceptJob(t *testing.T, repository *MemoryRepository, request Request, key string) Job {
	t.Helper()
	result, err := repository.Accept(context.Background(), AcceptCommand{
		Request:        request,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	return result.Job
}

func publishAccepted(t *testing.T, repository *MemoryRepository) Outbox {
	t.Helper()
	claims, err := repository.ClaimOutbox(context.Background(), ClaimOutboxCommand{
		Owner: "dispatcher-test",
		Limit: 1,
		TTL:   time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimOutbox: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("claimed %d outbox rows, want 1", len(claims))
	}
	published, err := repository.MarkOutboxPublished(context.Background(), claims[0].Outbox.ID, claims[0].Token)
	if err != nil {
		t.Fatalf("MarkOutboxPublished: %v", err)
	}
	return published
}

func acquireAndStart(t *testing.T, repository *MemoryRepository, job Job, agent string, ttl time.Duration) (Lease, LeaseRef) {
	t.Helper()
	lease, err := repository.AcquireLease(context.Background(), LeaseCommand{
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		AgentID:   agent,
		TTL:       ttl,
		TraceID:   "trace-" + agent,
	})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	reference := LeaseRef{
		ProjectID: job.ProjectID,
		JobID:     job.ID,
		AttemptID: lease.Attempt.ID,
		Token:     lease.Token,
		Fence:     lease.Fence,
	}
	if _, err := repository.Start(context.Background(), reference); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return lease, reference
}

func TestRequestIsCanonicalAndImmutable(t *testing.T) {
	t.Parallel()

	input := json.RawMessage(`{"z":2, "a":{"value":1}}`)
	request, err := NewRequest("project-a", FunctionRef{
		Name: "http.fetch", Version: "1", Digest: "sha256:one",
	}, input, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	input[2] = 'X'
	if got := string(request.Input()); got != `{"a":{"value":1},"z":2}` {
		t.Fatalf("canonical input = %s", got)
	}
	callerCopy := request.Input()
	callerCopy[2] = 'X'
	if got := string(request.Input()); got != `{"a":{"value":1},"z":2}` {
		t.Fatalf("accessor aliased immutable input: %s", got)
	}

	equivalent, err := NewRequest("project-a", FunctionRef{
		Name: "http.fetch", Version: "1", Digest: "sha256:one",
	}, json.RawMessage(`{"a":{"value":1},"z":2}`), RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if request.Digest() != equivalent.Digest() {
		t.Fatalf("canonical requests have different digests: %s != %s", request.Digest(), equivalent.Digest())
	}
	if _, err := NewRequest("project-a", FunctionRef{
		Name: "http.fetch", Version: "1", Digest: "sha256:one",
	}, json.RawMessage(`{} trailing`), RequestOptions{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid trailing JSON error = %v", err)
	}

	policy := RetryPolicy{InitialBackoff: 2 * time.Second, MaxBackoff: 5 * time.Second}
	if got := policy.Backoff(1); got != 2*time.Second {
		t.Fatalf("attempt 1 backoff = %v", got)
	}
	if got := policy.Backoff(3); got != 5*time.Second {
		t.Fatalf("bounded attempt 3 backoff = %v", got)
	}
}

func TestConcurrentAcceptanceIsProjectScopedAndIdempotent(t *testing.T) {
	repository, _ := newTestRepository()
	request := testRequest(t, "project-a", `{"url":"https://example.test"}`, 3)

	const callers = 64
	start := make(chan struct{})
	results := make(chan AcceptResult, callers)
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := repository.Accept(context.Background(), AcceptCommand{
				Request: request, IdempotencyKey: "same-key",
			})
			results <- result
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)

	var jobID string
	duplicates := 0
	for result := range results {
		if jobID == "" {
			jobID = result.Job.ID
		}
		if result.Job.ID != jobID {
			t.Fatalf("idempotent acceptance returned %q and %q", jobID, result.Job.ID)
		}
		if result.Duplicate {
			duplicates++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("Accept: %v", err)
		}
	}
	if duplicates != callers-1 {
		t.Fatalf("duplicate results = %d, want %d", duplicates, callers-1)
	}
	outbox, err := repository.OutboxRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(outbox) != 1 {
		t.Fatalf("outbox rows = %d, want one atomic acceptance row", len(outbox))
	}
	events, err := repository.Events(context.Background(), "project-a", jobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "job.accepted" {
		t.Fatalf("acceptance events = %#v", events)
	}

	changed := testRequest(t, "project-a", `{"url":"https://other.test"}`, 3)
	if _, err := repository.Accept(context.Background(), AcceptCommand{
		Request: changed, IdempotencyKey: "same-key",
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting idempotency key error = %v", err)
	}

	otherProject := testRequest(t, "project-b", `{"url":"https://example.test"}`, 3)
	other, err := repository.Accept(context.Background(), AcceptCommand{
		Request: otherProject, IdempotencyKey: "same-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if other.Job.ID == jobID {
		t.Fatal("idempotency key leaked across projects")
	}
}

func TestOutboxClaimsAreFencedAndAmbiguousPublishDeduplicates(t *testing.T) {
	repository, clock := newTestRepository()
	request := testRequest(t, "project-a", `{"url":"https://example.test"}`, 2)
	job := acceptJob(t, repository, request, "outbox")
	if job.State != StateAccepted {
		t.Fatalf("accepted state = %s", job.State)
	}

	first, err := repository.ClaimOutbox(context.Background(), ClaimOutboxCommand{
		Owner: "dispatcher-one", Limit: 1, TTL: time.Minute,
	})
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim = %#v, %v", first, err)
	}
	second, err := repository.ClaimOutbox(context.Background(), ClaimOutboxCommand{
		Owner: "dispatcher-two", Limit: 1, TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("active claim was stolen: %#v", second)
	}
	if _, err := repository.MarkOutboxPublished(context.Background(), first[0].Outbox.ID, "wrong-token"); !errors.Is(err, ErrOutboxClaimLost) {
		t.Fatalf("wrong-token acknowledgement error = %v", err)
	}

	publisher := NewMemoryPublisher()
	message := Message{
		ID: first[0].Outbox.MessageID, Topic: first[0].Outbox.Topic, Payload: first[0].Outbox.Payload,
	}
	if err := publisher.Publish(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	// Model publish success followed by a process crash before the database ack.
	clock.Advance(time.Minute)
	dispatcher := Dispatcher{
		Outbox: repository, Publisher: publisher, Owner: "dispatcher-two",
		BatchSize: 1, ClaimTTL: time.Minute,
	}
	report, err := dispatcher.DispatchOnce(context.Background())
	if err != nil {
		t.Fatalf("DispatchOnce: %v", err)
	}
	if report.Claimed != 1 || report.Published != 1 || report.Failed != 0 {
		t.Fatalf("dispatch report = %#v", report)
	}
	if len(publisher.Messages()) != 1 {
		t.Fatalf("duplicate deterministic message was stored %d times", len(publisher.Messages()))
	}
	if _, err := repository.MarkOutboxPublished(context.Background(), first[0].Outbox.ID, first[0].Token); !errors.Is(err, nil) {
		t.Fatalf("published acknowledgement should be idempotent: %v", err)
	}
	stored, err := repository.Get(context.Background(), "project-a", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != StateQueued {
		t.Fatalf("published job state = %s, want queued", stored.State)
	}
	rows, _ := repository.OutboxRecords(context.Background())
	if rows[0].MessageID != first[0].Outbox.MessageID || rows[0].PublishAttempts != 2 {
		t.Fatalf("outbox retry metadata = %#v", rows[0])
	}
}

func TestConcurrentLeaseAcquisitionAndRenewal(t *testing.T) {
	repository, _ := newTestRepository()
	request := testRequest(t, "project-a", `{}`, 3)
	job := acceptJob(t, repository, request, "lease-race")
	publishAccepted(t, repository)

	const agents = 64
	start := make(chan struct{})
	successes := make(chan Lease, agents)
	errs := make(chan error, agents)
	var wait sync.WaitGroup
	for index := range agents {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			lease, err := repository.AcquireLease(context.Background(), LeaseCommand{
				ProjectID: "project-a",
				JobID:     job.ID,
				AgentID:   fmt.Sprintf("agent-%d", index),
				TTL:       time.Minute,
			})
			if err == nil {
				successes <- lease
			} else {
				errs <- err
			}
		}()
	}
	close(start)
	wait.Wait()
	close(successes)
	close(errs)

	var lease Lease
	count := 0
	for value := range successes {
		lease = value
		count++
	}
	if count != 1 {
		t.Fatalf("successful leases = %d, want 1", count)
	}
	for err := range errs {
		if !errors.Is(err, ErrNotClaimable) {
			t.Fatalf("losing lease error = %v", err)
		}
	}
	attempts, _ := repository.Attempts(context.Background(), "project-a", job.ID)
	if len(attempts) != 1 || attempts[0].Number != 1 || attempts[0].Fence != 1 {
		t.Fatalf("attempts after lease race = %#v", attempts)
	}

	reference := LeaseRef{
		ProjectID: "project-a", JobID: job.ID, AttemptID: lease.Attempt.ID,
		Token: lease.Token, Fence: lease.Fence,
	}
	if _, err := repository.Start(context.Background(), reference); err != nil {
		t.Fatal(err)
	}
	bad := reference
	bad.Token = "not-the-token"
	if _, err := repository.RenewLease(context.Background(), RenewLeaseCommand{
		Lease: bad, TTL: time.Minute,
	}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("bad token renewal error = %v", err)
	}
	bad = reference
	bad.Fence++
	if _, err := repository.RenewLease(context.Background(), RenewLeaseCommand{
		Lease: bad, TTL: time.Minute,
	}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("bad fence renewal error = %v", err)
	}

	start = make(chan struct{})
	errs = make(chan error, agents)
	for range agents {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := repository.RenewLease(context.Background(), RenewLeaseCommand{
				Lease: reference, TTL: 2 * time.Minute,
			})
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("valid concurrent renewal: %v", err)
		}
	}
}

func TestExpiredLeaseRecoveryFencesOldOwnerAndExhausts(t *testing.T) {
	repository, clock := newTestRepository()
	job := acceptJob(t, repository, testRequest(t, "project-a", `{}`, 2), "recovery")
	publishAccepted(t, repository)
	firstLease, firstRef := acquireAndStart(t, repository, job, "agent-one", time.Minute)

	clock.Advance(time.Minute)
	if _, err := repository.RenewLease(context.Background(), RenewLeaseCommand{
		Lease: firstRef, TTL: time.Minute,
	}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired renewal error = %v", err)
	}
	recoveries, err := repository.RecoverExpiredLeases(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveries) != 1 || !recoveries[0].Requeued ||
		recoveries[0].Attempt.State != AttemptFailed ||
		recoveries[0].Attempt.Failure.Category != "agent_lost" {
		t.Fatalf("first recovery = %#v", recoveries)
	}
	if _, err := repository.Finalize(context.Background(), FinalizeCommand{
		Lease: firstRef, Outcome: StateSucceeded, Result: json.RawMessage(`{"stale":true}`),
	}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale finalization error = %v", err)
	}

	secondLease, secondRef := acquireAndStart(t, repository, recoveries[0].Job, "agent-two", time.Minute)
	if secondLease.Fence <= firstLease.Fence {
		t.Fatalf("new fence %d did not supersede %d", secondLease.Fence, firstLease.Fence)
	}
	clock.Advance(time.Minute)
	recoveries, err = repository.RecoverExpiredLeases(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveries) != 1 || !recoveries[0].Exhausted ||
		recoveries[0].Job.State != StateDeadLettered {
		t.Fatalf("exhausted recovery = %#v", recoveries)
	}
	if _, err := repository.Start(context.Background(), secondRef); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired second lease start error = %v", err)
	}
	attempts, _ := repository.Attempts(context.Background(), "project-a", job.ID)
	if len(attempts) != 2 || attempts[0].State != AttemptFailed || attempts[1].State != AttemptFailed {
		t.Fatalf("preserved attempts = %#v", attempts)
	}
	events, _ := repository.Events(context.Background(), "project-a", job.ID)
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
	}
}

func TestLeasedBeforeStartRecoversAsLeaseExpired(t *testing.T) {
	repository, clock := newTestRepository()
	job := acceptJob(t, repository, testRequest(t, "project-a", `{}`, 2), "prestart")
	publishAccepted(t, repository)
	lease, err := repository.AcquireLease(context.Background(), LeaseCommand{
		ProjectID: "project-a", JobID: job.ID, AgentID: "agent", TTL: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	recoveries, err := repository.RecoverExpiredLeases(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoveries) != 1 || recoveries[0].Attempt.ID != lease.Attempt.ID ||
		recoveries[0].Attempt.State != AttemptLeaseExpired ||
		recoveries[0].Attempt.Failure.Category != "lease_expired" {
		t.Fatalf("pre-start recovery = %#v", recoveries)
	}
}

func TestTerminalFinalizationIsIdempotentUnderConcurrency(t *testing.T) {
	repository, _ := newTestRepository()
	job := acceptJob(t, repository, testRequest(t, "project-a", `{}`, 2), "finalize")
	publishAccepted(t, repository)
	_, reference := acquireAndStart(t, repository, job, "agent", time.Minute)
	command := FinalizeCommand{
		Lease: reference, Outcome: StateSucceeded,
		Result: json.RawMessage(`{"z":2,"a":1}`),
	}

	const callers = 64
	start := make(chan struct{})
	results := make(chan FinalizeResult, callers)
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := repository.Finalize(context.Background(), command)
			results <- result
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)

	firstCommits := 0
	for result := range results {
		if result.Job.State != StateSucceeded || result.Attempt.State != AttemptSucceeded {
			t.Fatalf("finalized result = %#v", result)
		}
		if !result.Duplicate {
			firstCommits++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("idempotent finalization error = %v", err)
		}
	}
	if firstCommits != 1 {
		t.Fatalf("first commits = %d, want 1", firstCommits)
	}
	conflict := command
	conflict.Result = json.RawMessage(`{"a":9}`)
	if _, err := repository.Finalize(context.Background(), conflict); !errors.Is(err, ErrFinalizationConflict) {
		t.Fatalf("different repeated finalization error = %v", err)
	}
	events, _ := repository.Events(context.Background(), "project-a", job.ID)
	if got := events[len(events)-1].Type; got != "job.succeeded" {
		t.Fatalf("last event = %s", got)
	}
	var terminalEvents int
	for _, event := range events {
		if event.Type == "job.succeeded" {
			terminalEvents++
		}
	}
	if terminalEvents != 1 {
		t.Fatalf("terminal events = %d", terminalEvents)
	}
}

func TestRetrySchedulingPreservesAttemptsAndDeadLettersAtLimit(t *testing.T) {
	repository, clock := newTestRepository()
	job := acceptJob(t, repository, testRequest(t, "project-a", `{}`, 2), "retry")
	publishAccepted(t, repository)
	_, firstRef := acquireAndStart(t, repository, job, "agent-one", time.Minute)
	failure := &Failure{
		Category: "navigation_timeout", Code: "deadline", Retryable: true,
		Details: json.RawMessage(`{"z":2,"a":1}`),
	}
	first, err := repository.Finalize(context.Background(), FinalizeCommand{
		Lease: firstRef, Outcome: StateFailed, Failure: failure,
		Retry: &RetryDirective{After: 5 * time.Minute, Reason: "transient_navigation"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Job.State != StateRetryWait || first.Job.NextAttemptAt == nil ||
		first.Attempt.State != AttemptFailed || first.Attempt.Retry == nil {
		t.Fatalf("first retry result = %#v", first)
	}
	if due, err := repository.EnqueueDueRetries(context.Background(), 10); err != nil || len(due) != 0 {
		t.Fatalf("early due retries = %#v, %v", due, err)
	}
	clock.Advance(5 * time.Minute)
	due, err := repository.EnqueueDueRetries(context.Background(), 10)
	if err != nil || len(due) != 1 || due[0].State != StateQueued {
		t.Fatalf("due retries = %#v, %v", due, err)
	}

	_, secondRef := acquireAndStart(t, repository, due[0], "agent-two", time.Minute)
	second, err := repository.Finalize(context.Background(), FinalizeCommand{
		Lease: secondRef, Outcome: StateFailed, Failure: failure,
		Retry: &RetryDirective{After: time.Second, Reason: "still_transient"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Job.State != StateDeadLettered || second.Job.LastRetry == nil ||
		!second.Job.LastRetry.Exhausted || second.Job.CompletedAt == nil {
		t.Fatalf("dead-letter result = %#v", second.Job)
	}
	attempts, _ := repository.Attempts(context.Background(), "project-a", job.ID)
	if len(attempts) != 2 || attempts[0].Number != 1 || attempts[1].Number != 2 {
		t.Fatalf("attempt history = %#v", attempts)
	}

	otherRepository, _ := newTestRepository()
	otherJob := acceptJob(t, otherRepository, testRequest(t, "project-a", `{}`, 2), "nonretry")
	publishAccepted(t, otherRepository)
	_, otherRef := acquireAndStart(t, otherRepository, otherJob, "agent", time.Minute)
	if _, err := otherRepository.Finalize(context.Background(), FinalizeCommand{
		Lease: otherRef, Outcome: StateFailed,
		Failure: &Failure{Category: "invalid_request"},
		Retry:   &RetryDirective{After: time.Second},
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-retryable failure retry error = %v", err)
	}
}

func TestCancellationIsDurableAndFencesRunningWork(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		repository, _ := newTestRepository()
		job := acceptJob(t, repository, testRequest(t, "project-a", `{}`, 2), "cancel-accepted")
		canceled, err := repository.Cancel(context.Background(), "project-a", job.ID, "no longer needed")
		if err != nil {
			t.Fatal(err)
		}
		if canceled.Job.State != StateCanceled || canceled.CancellationMsg != nil {
			t.Fatalf("accepted cancellation = %#v", canceled)
		}
		again, err := repository.Cancel(context.Background(), "project-a", job.ID, "again")
		if err != nil || !again.Duplicate {
			t.Fatalf("duplicate cancellation = %#v, %v", again, err)
		}
		publishAccepted(t, repository)
		stored, _ := repository.Get(context.Background(), "project-a", job.ID)
		if stored.State != StateCanceled {
			t.Fatalf("late dispatch acknowledgement resurrected job to %s", stored.State)
		}
	})

	t.Run("running", func(t *testing.T) {
		repository, _ := newTestRepository()
		job := acceptJob(t, repository, testRequest(t, "project-a", `{}`, 2), "cancel-running")
		publishAccepted(t, repository)
		_, reference := acquireAndStart(t, repository, job, "agent", time.Minute)
		canceled, err := repository.Cancel(context.Background(), "project-a", job.ID, "operator request")
		if err != nil {
			t.Fatal(err)
		}
		if canceled.Job.State != StateCanceled || !canceled.WasRunning ||
			canceled.AgentID != "agent" || canceled.CancellationMsg == nil ||
			canceled.CancellationMsg.Kind != OutboxCancel {
			t.Fatalf("running cancellation = %#v", canceled)
		}
		if _, err := repository.RenewLease(context.Background(), RenewLeaseCommand{
			Lease: reference, TTL: time.Minute,
		}); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("renew after cancellation error = %v", err)
		}
		if _, err := repository.Finalize(context.Background(), FinalizeCommand{
			Lease: reference, Outcome: StateSucceeded, Result: json.RawMessage(`{}`),
		}); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("finalize after cancellation error = %v", err)
		}
		attempts, _ := repository.Attempts(context.Background(), "project-a", job.ID)
		if len(attempts) != 1 || attempts[0].State != AttemptCanceled {
			t.Fatalf("canceled attempts = %#v", attempts)
		}
		rows, _ := repository.OutboxRecords(context.Background())
		if len(rows) != 2 || rows[1].MessageID != canceled.CancellationMsg.MessageID {
			t.Fatalf("cancellation outbox = %#v", rows)
		}
	})
}

func TestFinalizeCancelRaceAlwaysLeavesOneTerminalOutcome(t *testing.T) {
	for iteration := range 32 {
		repository, _ := newTestRepository()
		key := fmt.Sprintf("race-%d", iteration)
		job := acceptJob(t, repository, testRequest(t, "project-a", `{}`, 1), key)
		publishAccepted(t, repository)
		_, reference := acquireAndStart(t, repository, job, "agent", time.Minute)

		start := make(chan struct{})
		var wait sync.WaitGroup
		var finalizeResult FinalizeResult
		var finalizeErr error
		var cancelResult CancelResult
		var cancelErr error
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			finalizeResult, finalizeErr = repository.Finalize(context.Background(), FinalizeCommand{
				Lease: reference, Outcome: StateSucceeded, Result: json.RawMessage(`{}`),
			})
		}()
		go func() {
			defer wait.Done()
			<-start
			cancelResult, cancelErr = repository.Cancel(context.Background(), "project-a", job.ID, "race")
		}()
		close(start)
		wait.Wait()

		if cancelErr != nil {
			t.Fatalf("iteration %d cancel: %v", iteration, cancelErr)
		}
		stored, err := repository.Get(context.Background(), "project-a", job.ID)
		if err != nil {
			t.Fatal(err)
		}
		switch stored.State {
		case StateSucceeded:
			if finalizeErr != nil || finalizeResult.Job.State != StateSucceeded || !cancelResult.Duplicate {
				t.Fatalf("iteration %d success winner: finalize=%#v/%v cancel=%#v", iteration, finalizeResult, finalizeErr, cancelResult)
			}
		case StateCanceled:
			if !errors.Is(finalizeErr, ErrLeaseLost) || cancelResult.Job.State != StateCanceled {
				t.Fatalf("iteration %d cancel winner: finalize err=%v cancel=%#v", iteration, finalizeErr, cancelResult)
			}
		default:
			t.Fatalf("iteration %d nonterminal race state %s", iteration, stored.State)
		}
		attempts, _ := repository.Attempts(context.Background(), "project-a", job.ID)
		if len(attempts) != 1 || !attempts[0].State.Terminal() {
			t.Fatalf("iteration %d attempt invariant = %#v", iteration, attempts)
		}
	}
}

func TestQueriesAreProjectScopedPaginatedAndDetached(t *testing.T) {
	repository, clock := newTestRepository()
	var projectA []Job
	for index := range 5 {
		job := acceptJob(t, repository, testRequest(t, "project-a", fmt.Sprintf(`{"index":%d}`, index), 1), fmt.Sprintf("a-%d", index))
		projectA = append(projectA, job)
		clock.Advance(time.Second)
	}
	other := acceptJob(t, repository, testRequest(t, "project-b", `{}`, 1), "b")
	if _, err := repository.Get(context.Background(), "project-a", other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project get error = %v", err)
	}
	if _, err := repository.Events(context.Background(), "project-a", other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project events error = %v", err)
	}

	var listed []Job
	cursor := ""
	for {
		page, err := repository.List(context.Background(), "project-a", ListOptions{
			Limit: 2, Cursor: cursor,
		})
		if err != nil {
			t.Fatal(err)
		}
		listed = append(listed, page.Jobs...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(listed) != len(projectA) {
		t.Fatalf("paginated jobs = %d, want %d", len(listed), len(projectA))
	}
	seen := make(map[string]bool)
	for _, job := range listed {
		if seen[job.ID] {
			t.Fatalf("duplicate paginated job %s", job.ID)
		}
		seen[job.ID] = true
	}
	if listed[0].ID != projectA[len(projectA)-1].ID {
		t.Fatalf("list is not newest first: first=%s newest=%s", listed[0].ID, projectA[len(projectA)-1].ID)
	}
	if _, err := repository.List(context.Background(), "project-a", ListOptions{Cursor: "not-base64"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("bad cursor error = %v", err)
	}

	snapshot, err := repository.Get(context.Background(), "project-a", projectA[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	input := snapshot.Request.Input()
	input[0] = 'X'
	snapshot.Result = json.RawMessage(`{"mutated":true}`)
	snapshot.State = StateFailed
	again, _ := repository.Get(context.Background(), "project-a", projectA[0].ID)
	if again.State != StateAccepted || string(again.Request.Input()) != `{"index":0}` || again.Result != nil {
		t.Fatalf("job query aliases repository memory: %#v", again)
	}
	events, _ := repository.Events(context.Background(), "project-a", projectA[0].ID)
	events[0].Payload = json.RawMessage(`{"mutated":true}`)
	eventsAgain, _ := repository.Events(context.Background(), "project-a", projectA[0].ID)
	if eventsAgain[0].Payload != nil {
		t.Fatalf("event query aliases repository memory: %#v", eventsAgain[0])
	}
}

func TestCanceledContextDoesNotMutateRepository(t *testing.T) {
	t.Parallel()
	repository, _ := newTestRepository()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.Accept(ctx, AcceptCommand{
		Request: testRequest(t, "project-a", `{}`, 1),
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acceptance error = %v", err)
	}
	rows, err := repository.OutboxRecords(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("canceled acceptance wrote outbox rows: %#v", rows)
	}
}
