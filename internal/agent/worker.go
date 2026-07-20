package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dagflows/neurun-io/internal/function"
	"github.com/dagflows/neurun-io/internal/ids"
	"github.com/dagflows/neurun-io/internal/job"
	"github.com/dagflows/neurun-io/internal/queue"
)

type Worker struct {
	repository      job.Repository
	consumer        queue.Consumer
	invocations     *function.Service
	agentID         string
	capabilities    []string
	concurrency     int
	leaseTTL        time.Duration
	finalizeTimeout time.Duration
	logger          *slog.Logger

	activeMu sync.Mutex
	active   map[string]activeAttempt
}

type Options struct {
	AgentID         string
	Capabilities    []string
	Concurrency     int
	LeaseTTL        time.Duration
	FinalizeTimeout time.Duration
	Logger          *slog.Logger
}

func NewWorker(
	repository job.Repository,
	consumer queue.Consumer,
	invocations *function.Service,
	options Options,
) (*Worker, error) {
	if repository == nil || consumer == nil || invocations == nil {
		return nil, errors.New("agent requires repository, queue consumer, and invocation service")
	}
	if options.AgentID == "" {
		return nil, errors.New("agent ID is required")
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 1
	}
	if options.LeaseTTL <= 0 {
		options.LeaseTTL = 30 * time.Second
	}
	if options.FinalizeTimeout <= 0 {
		options.FinalizeTimeout = 5 * time.Second
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Worker{
		repository:      repository,
		consumer:        consumer,
		invocations:     invocations,
		agentID:         options.AgentID,
		capabilities:    append([]string(nil), options.Capabilities...),
		concurrency:     options.Concurrency,
		leaseTTL:        options.LeaseTTL,
		finalizeTimeout: options.FinalizeTimeout,
		logger:          options.Logger,
		active:          make(map[string]activeAttempt),
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	slots := make(chan struct{}, w.concurrency)
	var active sync.WaitGroup
	defer active.Wait()

	for {
		delivery, err := w.consumer.Next(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}

		// Cancellation subjects bypass execution capacity so a saturated worker
		// can still stop a running attempt. Production brokers should also use a
		// separate control subscription; this preserves that behavior in the
		// all-in-one in-memory runtime.
		if isCancellationTopic(delivery.Message().Topic) {
			if err := w.handleDelivery(ctx, delivery); err != nil {
				w.logDeliveryError(delivery, err)
			}
			continue
		}

		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			_ = delivery.Nack()
			return nil
		}

		active.Add(1)
		go func() {
			defer active.Done()
			defer func() { <-slots }()
			if err := w.handleDelivery(ctx, delivery); err != nil {
				w.logDeliveryError(delivery, err)
			}
		}()
	}
}

func (w *Worker) ProcessOne(ctx context.Context) error {
	delivery, err := w.consumer.Next(ctx)
	if err != nil {
		return err
	}
	return w.handleDelivery(ctx, delivery)
}

type dispatchEnvelope struct {
	MessageID      string          `json:"message_id"`
	JobID          string          `json:"job_id"`
	ProjectID      string          `json:"project_id"`
	RequestDigest  string          `json:"request_digest"`
	DispatchNumber uint64          `json:"dispatch_number"`
	Function       job.FunctionRef `json:"function"`
}

func (w *Worker) handleDelivery(ctx context.Context, delivery queue.Delivery) error {
	message := delivery.Message()
	if isCancellationTopic(message.Topic) {
		return w.handleCancellation(delivery, message)
	}
	return w.handleDispatch(ctx, delivery, message)
}

func (w *Worker) handleDispatch(
	ctx context.Context,
	delivery queue.Delivery,
	message job.Message,
) error {
	envelope, err := decodeDispatch(message.Payload)
	if err != nil {
		return settleDelivery(
			delivery,
			true,
			fmt.Errorf("decode dispatch %s: %w", message.ID, err),
		)
	}
	if envelope.MessageID != message.ID {
		return settleDelivery(delivery, true, fmt.Errorf(
			"dispatch message ID mismatch: envelope=%s transport=%s",
			envelope.MessageID,
			message.ID,
		))
	}

	traceID, err := ids.Trace()
	if err != nil {
		return settleDelivery(delivery, false, err)
	}
	lease, err := w.repository.AcquireLease(ctx, job.LeaseCommand{
		ProjectID: envelope.ProjectID,
		JobID:     envelope.JobID,
		AgentID:   w.agentID,
		TTL:       w.leaseTTL,
		TraceID:   traceID,
	})
	if err != nil {
		switch {
		case errors.Is(err, job.ErrNotClaimable),
			errors.Is(err, job.ErrAttemptsExhausted),
			errors.Is(err, job.ErrNotFound):
			return settleDelivery(delivery, true, nil)
		default:
			return settleDelivery(delivery, false, fmt.Errorf("acquire job lease: %w", err))
		}
	}
	reference := job.LeaseRef{
		ProjectID: envelope.ProjectID,
		JobID:     envelope.JobID,
		AttemptID: lease.Attempt.ID,
		Token:     lease.Token,
		Fence:     lease.Fence,
	}
	if _, err := w.repository.Start(ctx, reference); err != nil {
		if errors.Is(err, job.ErrLeaseLost) {
			return settleDelivery(delivery, true, nil)
		}
		return settleDelivery(delivery, false, fmt.Errorf("start job attempt: %w", err))
	}

	executionCtx, cancel := context.WithCancel(ctx)
	running := activeAttempt{
		projectID: envelope.ProjectID,
		jobID:     envelope.JobID,
		attemptID: lease.Attempt.ID,
		fence:     lease.Fence,
		cancel:    cancel,
	}
	if err := w.registerActive(running); err != nil {
		cancel()
		return w.finalizeAndSettle(
			ctx,
			delivery,
			lease,
			function.Invocation{},
			nil,
			err,
		)
	}

	// Establish both ownership heartbeats immediately. Besides avoiding a
	// short initial visibility window, the lease CAS closes the race where a
	// cancellation was committed just before the attempt entered active.
	if _, err := w.repository.RenewLease(executionCtx, job.RenewLeaseCommand{
		Lease: reference,
		TTL:   w.leaseTTL,
	}); err != nil {
		w.unregisterActive(running)
		cancel()
		if errors.Is(err, job.ErrLeaseLost) {
			return settleDelivery(delivery, true, nil)
		}
		return w.finalizeAndSettle(
			ctx,
			delivery,
			lease,
			function.Invocation{},
			nil,
			fmt.Errorf("establish job lease heartbeat: %w", err),
		)
	}
	if err := delivery.Extend(w.leaseTTL); err != nil {
		w.unregisterActive(running)
		cancel()
		return w.finalizeAndSettle(
			ctx,
			delivery,
			lease,
			function.Invocation{},
			nil,
			fmt.Errorf("establish queue visibility heartbeat: %w", err),
		)
	}

	renewalDone := make(chan struct{})
	renewalResult := make(chan error, 1)
	go w.renew(executionCtx, reference, delivery, renewalDone, renewalResult, cancel)

	invocation, invokeErr := w.invocations.Invoke(executionCtx, function.InvocationRequest{
		ProjectID: envelope.ProjectID,
		Function: function.FunctionRef{
			Name:    lease.Job.Request.Function().Name,
			Version: lease.Job.Request.Function().Version,
			Digest:  lease.Job.Request.Function().Digest,
		},
		Context: &function.ExecutionContext{
			ProjectID:     envelope.ProjectID,
			JobID:         envelope.JobID,
			AttemptID:     lease.Attempt.ID,
			EphemeralHTTP: true,
			Capabilities:  append([]string(nil), w.capabilities...),
		},
		Input:   lease.Job.Request.Input(),
		TraceID: traceID,
	})
	close(renewalDone)
	renewalErr := <-renewalResult
	w.unregisterActive(running)
	cancel()

	if errors.Is(renewalErr, job.ErrLeaseLost) {
		// A cancellation, recovery, or newer fenced owner has already made the
		// durable decision. This delivery must not execute or be redelivered.
		return settleDelivery(delivery, true, nil)
	}

	var interruption error
	if renewalErr != nil {
		finishedBeforeShutdown := ctx.Err() != nil &&
			errors.Is(renewalErr, context.Canceled) &&
			invocation.Status != "" &&
			invocation.Status != function.InvocationCanceled
		if !finishedBeforeShutdown {
			interruption = fmt.Errorf("renew execution ownership: %w", renewalErr)
		}
	} else if ctx.Err() != nil && invocation.Status == function.InvocationCanceled {
		interruption = ctx.Err()
	}
	return w.finalizeAndSettle(
		ctx,
		delivery,
		lease,
		invocation,
		invokeErr,
		interruption,
	)
}

func (w *Worker) finalizeAndSettle(
	ctx context.Context,
	delivery queue.Delivery,
	lease job.Lease,
	invocation function.Invocation,
	invokeErr error,
	interruption error,
) error {
	finalize := finalizationFor(lease, invocation, invokeErr, interruption)
	finalizeCtx, finalizeCancel := context.WithTimeout(context.WithoutCancel(ctx), w.finalizeTimeout)
	defer finalizeCancel()
	if _, err := w.repository.Finalize(finalizeCtx, finalize); err != nil {
		if errors.Is(err, job.ErrLeaseLost) {
			return settleDelivery(delivery, true, nil)
		}
		return settleDelivery(delivery, false, fmt.Errorf("finalize job attempt: %w", err))
	}
	return settleDelivery(delivery, true, nil)
}

func (w *Worker) renew(
	ctx context.Context,
	reference job.LeaseRef,
	delivery queue.Delivery,
	done <-chan struct{},
	result chan<- error,
	cancel context.CancelFunc,
) {
	interval := w.leaseTTL / 3
	if interval <= 0 {
		interval = time.Nanosecond
	}
	timer := time.NewTicker(interval)
	defer timer.Stop()
	for {
		select {
		case <-done:
			result <- nil
			return
		case <-ctx.Done():
			result <- ctx.Err()
			return
		case <-timer.C:
			if _, err := w.repository.RenewLease(ctx, job.RenewLeaseCommand{
				Lease: reference,
				TTL:   w.leaseTTL,
			}); err != nil {
				cancel()
				result <- fmt.Errorf("renew job lease: %w", err)
				return
			}
			if err := delivery.Extend(w.leaseTTL); err != nil {
				cancel()
				result <- fmt.Errorf("extend queue visibility: %w", err)
				return
			}
		}
	}
}

func finalizationFor(
	lease job.Lease,
	invocation function.Invocation,
	invokeErr error,
	interruption error,
) job.FinalizeCommand {
	outcome := job.StateSucceeded
	switch invocation.Status {
	case function.InvocationSucceeded:
		outcome = job.StateSucceeded
	case function.InvocationRejected:
		outcome = job.StateRejected
	case function.InvocationCanceled:
		outcome = job.StateCanceled
	default:
		outcome = job.StateFailed
	}

	var failure *job.Failure
	if invocation.Failure != nil {
		details, _ := json.Marshal(invocation.Failure.Details)
		failure = &job.Failure{
			Category:  string(invocation.Failure.Category),
			Code:      invocation.Failure.Code,
			Message:   invocation.Failure.Message,
			Retryable: invocation.Failure.Retryable,
			Details:   details,
		}
	} else if invokeErr != nil {
		failure = &job.Failure{
			Category: "internal_error",
			Code:     "invocation_failed",
			Message:  "atomic-function invocation did not produce failure metadata",
		}
		outcome = job.StateFailed
	}
	if interruption != nil {
		failure = &job.Failure{
			Category:  "agent_lost",
			Code:      "execution_ownership_lost",
			Message:   "agent lost execution ownership before finalization",
			Retryable: true,
		}
		outcome = job.StateFailed
	}
	if outcome != job.StateSucceeded && outcome != job.StateCanceled && failure == nil {
		failure = &job.Failure{
			Category: "internal_error",
			Code:     "missing_invocation_failure",
			Message:  "atomic-function invocation did not produce terminal failure metadata",
		}
		outcome = job.StateFailed
	}

	command := job.FinalizeCommand{
		Lease: job.LeaseRef{
			ProjectID: lease.Job.ProjectID,
			JobID:     lease.Job.ID,
			AttemptID: lease.Attempt.ID,
			Token:     lease.Token,
			Fence:     lease.Fence,
		},
		Outcome: outcome,
		Failure: failure,
	}
	if interruption == nil {
		command.Result = append(json.RawMessage(nil), invocation.Output...)
	}
	if failure != nil && failure.Retryable &&
		(outcome == job.StateFailed || outcome == job.StateRejected) {
		command.Retry = &job.RetryDirective{
			After:  lease.Job.Request.RetryPolicy().Backoff(lease.Attempt.Number),
			Reason: failure.Category,
		}
	}
	return command
}

type cancellationEnvelope struct {
	MessageID string `json:"message_id"`
	JobID     string `json:"job_id"`
	ProjectID string `json:"project_id"`
	AttemptID string `json:"attempt_id"`
	Fence     uint64 `json:"fence"`
	Reason    string `json:"reason"`
}

type activeAttempt struct {
	projectID string
	jobID     string
	attemptID string
	fence     uint64
	cancel    context.CancelFunc
}

func (w *Worker) handleCancellation(delivery queue.Delivery, message job.Message) error {
	envelope, err := decodeCancellation(message.Payload)
	if err != nil {
		return settleDelivery(
			delivery,
			true,
			fmt.Errorf("decode cancellation %s: %w", message.ID, err),
		)
	}
	if envelope.MessageID != message.ID {
		return settleDelivery(delivery, true, fmt.Errorf(
			"cancellation message ID mismatch: envelope=%s transport=%s",
			envelope.MessageID,
			message.ID,
		))
	}
	if message.Topic != "jobs.cancel."+envelope.JobID {
		return settleDelivery(delivery, true, fmt.Errorf(
			"cancellation topic mismatch: topic=%s job_id=%s",
			message.Topic,
			envelope.JobID,
		))
	}

	w.cancelActive(envelope)
	return settleDelivery(delivery, true, nil)
}

func (w *Worker) registerActive(attempt activeAttempt) error {
	key := activeKey(attempt.projectID, attempt.jobID)
	w.activeMu.Lock()
	defer w.activeMu.Unlock()
	if current, exists := w.active[key]; exists {
		return fmt.Errorf(
			"job %s already has active attempt %s at fence %d",
			attempt.jobID,
			current.attemptID,
			current.fence,
		)
	}
	w.active[key] = attempt
	return nil
}

func (w *Worker) unregisterActive(attempt activeAttempt) {
	key := activeKey(attempt.projectID, attempt.jobID)
	w.activeMu.Lock()
	current, exists := w.active[key]
	if exists && current.attemptID == attempt.attemptID && current.fence == attempt.fence {
		delete(w.active, key)
	}
	w.activeMu.Unlock()
}

func (w *Worker) cancelActive(cancellation cancellationEnvelope) {
	key := activeKey(cancellation.ProjectID, cancellation.JobID)
	w.activeMu.Lock()
	attempt, exists := w.active[key]
	matches := exists &&
		attempt.attemptID == cancellation.AttemptID &&
		attempt.fence == cancellation.Fence
	cancel := attempt.cancel
	w.activeMu.Unlock()
	if matches {
		cancel()
	}
}

func (w *Worker) logDeliveryError(delivery queue.Delivery, err error) {
	w.logger.Error("agent delivery failed",
		"agent_id", w.agentID,
		"message_id", delivery.Message().ID,
		"error", err,
	)
}

func activeKey(projectID, jobID string) string {
	return projectID + "\x00" + jobID
}

func isCancellationTopic(topic string) bool {
	return strings.HasPrefix(topic, "jobs.cancel.")
}

func settleDelivery(delivery queue.Delivery, acknowledge bool, cause error) error {
	var err error
	action := "nack"
	if acknowledge {
		action = "ack"
		err = delivery.Ack()
	} else {
		err = delivery.Nack()
	}
	if err != nil {
		return errors.Join(cause, fmt.Errorf("%s queue delivery: %w", action, err))
	}
	return cause
}

func decodeDispatch(raw []byte) (dispatchEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope dispatchEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return dispatchEnvelope{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return dispatchEnvelope{}, errors.New("multiple JSON values")
		}
		return dispatchEnvelope{}, err
	}
	if envelope.MessageID == "" || envelope.JobID == "" || envelope.ProjectID == "" {
		return dispatchEnvelope{}, errors.New("message_id, job_id, and project_id are required")
	}
	return envelope, nil
}

func decodeCancellation(raw []byte) (cancellationEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope cancellationEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return cancellationEnvelope{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return cancellationEnvelope{}, errors.New("multiple JSON values")
		}
		return cancellationEnvelope{}, err
	}
	if envelope.MessageID == "" ||
		envelope.JobID == "" ||
		envelope.ProjectID == "" ||
		envelope.AttemptID == "" ||
		envelope.Fence == 0 {
		return cancellationEnvelope{}, errors.New(
			"message_id, job_id, project_id, attempt_id, and fence are required",
		)
	}
	return envelope, nil
}
