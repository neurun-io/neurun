package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dagflows/neurun-io/internal/function"
	"github.com/dagflows/neurun-io/internal/job"
	"github.com/dagflows/neurun-io/internal/queue"
)

func TestWorkerExecutesDurableFunctionJob(t *testing.T) {
	t.Parallel()

	registry := function.NewRegistry()
	if err := function.RegisterBuiltins(registry); err != nil {
		t.Fatal(err)
	}
	resolved, _, err := registry.ResolveRef(function.FunctionRef{Name: "system.echo", Version: "1"})
	if err != nil {
		t.Fatal(err)
	}
	invocations := function.NewService(registry, function.NewMemoryStore())

	repository := job.NewMemoryRepository()
	request, err := job.NewRequest("prj_1", job.FunctionRef{
		Name: resolved.Name, Version: resolved.Version, Digest: resolved.Digest,
	}, json.RawMessage(`{"message":"hello"}`), job.RequestOptions{MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := repository.Accept(context.Background(), job.AcceptCommand{
		Request: request, IdempotencyKey: "idem_worker",
	})
	if err != nil {
		t.Fatal(err)
	}

	broker := queue.NewMemoryBroker(time.Second)
	report, err := (job.Dispatcher{
		Outbox: repository, Publisher: broker, Owner: "dispatcher_1",
	}).DispatchOnce(context.Background())
	if err != nil || report.Published != 1 {
		t.Fatalf("dispatch report=%#v err=%v", report, err)
	}

	worker, err := NewWorker(repository, broker, invocations, Options{
		AgentID: "agt_1", Concurrency: 1, LeaseTTL: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}

	completed, err := repository.Get(context.Background(), "prj_1", accepted.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != job.StateSucceeded {
		t.Fatalf("state = %s, failure = %#v", completed.State, completed.LastFailure)
	}
	var output map[string]any
	if err := json.Unmarshal(completed.Result, &output); err != nil {
		t.Fatal(err)
	}
	if output["message"] != "hello" {
		t.Fatalf("output = %#v", output)
	}
	attempts, err := repository.Attempts(context.Background(), "prj_1", accepted.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].State != job.AttemptSucceeded {
		t.Fatalf("attempts = %#v", attempts)
	}
}

func TestWorkerRenewsLeaseAndQueueVisibilityDuringExecution(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	invocations, reference := testInvocationService(t, "test.wait_renew", func(
		ctx context.Context,
		_ *function.ExecutionContext,
		_ json.RawMessage,
	) (function.FunctionResult, error) {
		close(started)
		select {
		case <-release:
			return function.FunctionResult{Output: json.RawMessage(`{"ok":true}`)}, nil
		case <-ctx.Done():
			return function.FunctionResult{}, ctx.Err()
		}
	})

	repository := job.NewMemoryRepository()
	broker := queue.NewMemoryBroker(40 * time.Millisecond)
	accepted := acceptAndDispatch(t, repository, broker, reference, 1)
	worker, err := NewWorker(repository, broker, invocations, Options{
		AgentID: "agt_renew", Concurrency: 1, LeaseTTL: 90 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		result <- worker.ProcessOne(context.Background())
	}()
	awaitSignal(t, started, "function start")

	// Wait beyond both the broker's original visibility window and the job
	// lease. A competing consumer must not see the in-flight message.
	time.Sleep(130 * time.Millisecond)
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer probeCancel()
	if _, err := broker.Next(probeCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("in-flight delivery was redelivered: %v", err)
	}

	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not complete")
	}

	stored, err := repository.Get(context.Background(), "prj_1", accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != job.StateSucceeded {
		t.Fatalf("state = %s, failure = %#v", stored.State, stored.LastFailure)
	}
	events, err := repository.Events(context.Background(), "prj_1", accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	renewals := 0
	for _, event := range events {
		if event.Type == "job.lease_renewed" {
			renewals++
		}
	}
	if renewals < 3 {
		t.Fatalf("lease renewal events = %d, want at least 3", renewals)
	}
}

func TestWorkerFinalizesRetryableAgentLossDuringShutdown(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	invocations, reference := testInvocationService(t, "test.wait_shutdown", func(
		ctx context.Context,
		_ *function.ExecutionContext,
		_ json.RawMessage,
	) (function.FunctionResult, error) {
		close(started)
		<-ctx.Done()
		return function.FunctionResult{}, ctx.Err()
	})

	repository := job.NewMemoryRepository()
	broker := queue.NewMemoryBroker(time.Second)
	accepted := acceptAndDispatch(t, repository, broker, reference, 2)
	worker, err := NewWorker(repository, broker, invocations, Options{
		AgentID: "agt_shutdown", Concurrency: 1, LeaseTTL: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- worker.ProcessOne(ctx)
	}()
	awaitSignal(t, started, "function start")
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not finalize during shutdown")
	}

	stored, err := repository.Get(context.Background(), "prj_1", accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != job.StateRetryWait {
		t.Fatalf("state = %s, want %s", stored.State, job.StateRetryWait)
	}
	if stored.LastFailure == nil ||
		stored.LastFailure.Category != "agent_lost" ||
		!stored.LastFailure.Retryable {
		t.Fatalf("last failure = %#v", stored.LastFailure)
	}
	if stored.NextAttemptAt == nil {
		t.Fatal("shutdown retry was not scheduled")
	}
	if pending := broker.Pending(); pending != 0 {
		t.Fatalf("pending deliveries = %d, want 0", pending)
	}
}

func TestWorkerRoutesRunningJobCancellationAtCapacity(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	stopped := make(chan struct{})
	invocations, reference := testInvocationService(t, "test.wait_cancel", func(
		ctx context.Context,
		_ *function.ExecutionContext,
		_ json.RawMessage,
	) (function.FunctionResult, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return function.FunctionResult{}, ctx.Err()
	})

	repository := job.NewMemoryRepository()
	broker := queue.NewMemoryBroker(time.Second)
	accepted := acceptAndDispatch(t, repository, broker, reference, 1)
	worker, err := NewWorker(repository, broker, invocations, Options{
		AgentID: "agt_cancel", Concurrency: 1, LeaseTTL: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	runCtx, stopRun := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() {
		runResult <- worker.Run(runCtx)
	}()
	awaitSignal(t, started, "function start")

	canceled, err := repository.Cancel(
		context.Background(),
		"prj_1",
		accepted.ID,
		"user requested cancellation",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !canceled.WasRunning || canceled.CancellationMsg == nil {
		t.Fatalf("cancel result = %#v", canceled)
	}
	report, err := (job.Dispatcher{
		Outbox: repository, Publisher: broker, Owner: "dispatcher_cancel",
	}).DispatchOnce(context.Background())
	if err != nil || report.Published != 1 {
		t.Fatalf("dispatch report=%#v err=%v", report, err)
	}

	awaitSignal(t, stopped, "function cancellation")
	awaitCondition(t, "delivery settlement", func() bool {
		return broker.Pending() == 0
	})

	attempts, err := repository.Attempts(context.Background(), "prj_1", accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].State != job.AttemptCanceled {
		t.Fatalf("attempts = %#v", attempts)
	}

	stopRun()
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker Run did not stop")
	}
}

func TestWorkerCancellationRequiresMatchingAttemptFence(t *testing.T) {
	t.Parallel()

	canceled := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &Worker{
		active: map[string]activeAttempt{
			activeKey("prj_1", "job_1"): {
				projectID: "prj_1",
				jobID:     "job_1",
				attemptID: "att_1",
				fence:     7,
				cancel:    cancel,
			},
		},
	}
	go func() {
		<-ctx.Done()
		close(canceled)
	}()

	worker.cancelActive(cancellationEnvelope{
		ProjectID: "prj_1",
		JobID:     "job_1",
		AttemptID: "att_1",
		Fence:     6,
	})
	select {
	case <-canceled:
		t.Fatal("stale cancellation fence stopped the active attempt")
	case <-time.After(30 * time.Millisecond):
	}

	worker.cancelActive(cancellationEnvelope{
		ProjectID: "prj_1",
		JobID:     "job_1",
		AttemptID: "att_1",
		Fence:     7,
	})
	awaitSignal(t, canceled, "matching fenced cancellation")
}

func TestWorkerAcknowledgesPoisonAndNacksTransientLeaseFailure(t *testing.T) {
	t.Parallel()

	invocations := function.NewService(function.NewRegistry(), function.NewMemoryStore())

	t.Run("poison is acknowledged", func(t *testing.T) {
		broker := queue.NewMemoryBroker(time.Second)
		if err := broker.Publish(context.Background(), job.Message{
			ID: "msg_poison", Topic: "jobs.execute", Payload: []byte(`not-json`),
		}); err != nil {
			t.Fatal(err)
		}
		worker, err := NewWorker(
			job.NewMemoryRepository(),
			broker,
			invocations,
			Options{AgentID: "agt_poison"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessOne(context.Background()); err == nil {
			t.Fatal("poison delivery should report its decode failure")
		}
		if pending := broker.Pending(); pending != 0 {
			t.Fatalf("poison pending = %d, want 0", pending)
		}
	})

	t.Run("transient lease failure is nacked", func(t *testing.T) {
		sentinel := errors.New("database unavailable")
		repository := &failingAcquireRepository{
			Repository: job.NewMemoryRepository(),
			err:        sentinel,
		}
		broker := queue.NewMemoryBroker(time.Second)
		payload, err := json.Marshal(dispatchEnvelope{
			MessageID: "msg_retry",
			JobID:     "job_1",
			ProjectID: "prj_1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := broker.Publish(context.Background(), job.Message{
			ID: "msg_retry", Topic: "jobs.execute", Payload: payload,
		}); err != nil {
			t.Fatal(err)
		}
		worker, err := NewWorker(
			repository,
			broker,
			invocations,
			Options{AgentID: "agt_retry"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.ProcessOne(context.Background()); !errors.Is(err, sentinel) {
			t.Fatalf("ProcessOne error = %v", err)
		}

		redelivered, err := broker.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if redelivered.Message().ID != "msg_retry" {
			t.Fatalf("redelivered message = %#v", redelivered.Message())
		}
		if err := redelivered.Ack(); err != nil {
			t.Fatal(err)
		}
	})
}

type failingAcquireRepository struct {
	job.Repository
	err error
}

func (repository *failingAcquireRepository) AcquireLease(
	context.Context,
	job.LeaseCommand,
) (job.Lease, error) {
	return job.Lease{}, repository.err
}

func testInvocationService(
	t *testing.T,
	name string,
	execute function.Executor,
) (*function.Service, job.FunctionRef) {
	t.Helper()

	atomic, err := function.NewAtomicFunction(function.Manifest{
		Name:             name,
		Version:          "1",
		Category:         "test",
		Description:      "Test-only blocking function.",
		ExecutionContext: function.ExecutionContextNone,
		SideEffects:      function.SideEffectPure,
		Timeout: function.TimeoutPolicy{
			DefaultMS: 2000,
			MaximumMS: 5000,
		},
		InputSchema:  function.Schema{},
		OutputSchema: function.Schema{},
		Retry: function.RetryPolicy{AllowedFailures: []function.FailureCategory{
			function.FailureAgentLost,
		}},
	}, execute)
	if err != nil {
		t.Fatal(err)
	}
	registry := function.NewRegistry()
	if err := registry.Register(atomic); err != nil {
		t.Fatal(err)
	}
	resolved, _, err := registry.ResolveRef(function.FunctionRef{
		Name: name, Version: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return function.NewService(registry, function.NewMemoryStore()), job.FunctionRef{
		Name: resolved.Name, Version: resolved.Version, Digest: resolved.Digest,
	}
}

func acceptAndDispatch(
	t *testing.T,
	repository *job.MemoryRepository,
	broker *queue.MemoryBroker,
	reference job.FunctionRef,
	maxAttempts int,
) job.Job {
	t.Helper()

	request, err := job.NewRequest(
		"prj_1",
		reference,
		json.RawMessage(`{}`),
		job.RequestOptions{MaxAttempts: maxAttempts},
	)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := repository.Accept(context.Background(), job.AcceptCommand{
		Request: request, IdempotencyKey: "idem_" + reference.Name,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := (job.Dispatcher{
		Outbox: repository, Publisher: broker, Owner: "dispatcher_" + reference.Name,
	}).DispatchOnce(context.Background())
	if err != nil || report.Published != 1 {
		t.Fatalf("dispatch report=%#v err=%v", report, err)
	}
	return accepted.Job
}

func awaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func awaitCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(time.Millisecond)
	}
}
