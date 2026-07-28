package function

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type InvocationStatus string

const (
	InvocationAccepted  InvocationStatus = "accepted"
	InvocationRunning   InvocationStatus = "running"
	InvocationSucceeded InvocationStatus = "succeeded"
	InvocationRejected  InvocationStatus = "rejected"
	InvocationFailed    InvocationStatus = "failed"
	InvocationTimedOut  InvocationStatus = "timed_out"
	InvocationCanceled  InvocationStatus = "canceled"
)

var (
	ErrInvocationNotFound = errors.New("function invocation not found")
	ErrInvocationExists   = errors.New("function invocation already exists")
	ErrInvocationNotLive  = errors.New("function invocation is not running")
)

type InvocationRequest struct {
	ProjectID string            `json:"project_id,omitempty"`
	Function  FunctionRef       `json:"function"`
	Context   *ExecutionContext `json:"context,omitempty"`
	Input     json.RawMessage   `json:"input"`
	TimeoutMS int64             `json:"timeout_ms,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
}

// Invocation is an immutable snapshot returned from the in-memory store.
type Invocation struct {
	ID                string              `json:"invocation_id"`
	ProjectID         string              `json:"project_id,omitempty"`
	Function          FunctionRef         `json:"function"`
	Status            InvocationStatus    `json:"status"`
	SideEffects       SideEffectClass     `json:"side_effect_class,omitempty"`
	InputHash         string              `json:"input_hash,omitempty"`
	RedactedInput     json.RawMessage     `json:"redacted_input,omitempty"`
	Output            json.RawMessage     `json:"output,omitempty"`
	OutputSchemaValid bool                `json:"output_schema_valid"`
	Failure           *Failure            `json:"failure,omitempty"`
	Usage             Usage               `json:"usage"`
	Artifacts         []ArtifactReference `json:"artifacts,omitempty"`
	TraceID           string              `json:"trace_id"`
	SpanID            string              `json:"span_id"`
	Context           *ExecutionContext   `json:"context,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	StartedAt         *time.Time          `json:"started_at,omitempty"`
	FinishedAt        *time.Time          `json:"finished_at,omitempty"`
}

// InvocationError reports a failed invocation while preserving its generated
// ID for status and trace lookup.
type InvocationError struct {
	InvocationID string
	Failure      Failure
}

func (e *InvocationError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("invocation %s: %s", e.InvocationID, e.Failure.Message)
}

// InvocationStore is the persistence boundary used by Service.
type InvocationStore interface {
	Create(Invocation) error
	Save(Invocation) error
	Get(string) (Invocation, error)
	List() []Invocation
}

// MemoryStore is a concurrency-safe development implementation.
type MemoryStore struct {
	mu          sync.RWMutex
	invocations map[string]Invocation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{invocations: make(map[string]Invocation)}
}

func (s *MemoryStore) Create(invocation Invocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.invocations[invocation.ID]; exists {
		return fmt.Errorf("%w: %s", ErrInvocationExists, invocation.ID)
	}
	s.invocations[invocation.ID] = cloneInvocation(invocation)
	return nil
}

func (s *MemoryStore) Save(invocation Invocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.invocations[invocation.ID]; !exists {
		return fmt.Errorf("%w: %s", ErrInvocationNotFound, invocation.ID)
	}
	s.invocations[invocation.ID] = cloneInvocation(invocation)
	return nil
}

func (s *MemoryStore) Get(id string) (Invocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	invocation, exists := s.invocations[id]
	if !exists {
		return Invocation{}, fmt.Errorf("%w: %s", ErrInvocationNotFound, id)
	}
	return cloneInvocation(invocation), nil
}

func (s *MemoryStore) List() []Invocation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	invocations := make([]Invocation, 0, len(s.invocations))
	for _, invocation := range s.invocations {
		invocations = append(invocations, cloneInvocation(invocation))
	}
	sort.Slice(invocations, func(i, j int) bool {
		if !invocations[i].CreatedAt.Equal(invocations[j].CreatedAt) {
			return invocations[i].CreatedAt.Before(invocations[j].CreatedAt)
		}
		return invocations[i].ID < invocations[j].ID
	})
	return invocations
}

type ServiceOption func(*Service)

// WithClock replaces wall time for deterministic tests.
func WithClock(clock func() time.Time) ServiceOption {
	return func(service *Service) {
		if clock != nil {
			service.now = clock
		}
	}
}

// WithIDGenerator replaces opaque ID generation. The callback receives a
// prefix such as "fni", "trace", or "span".
func WithIDGenerator(generator func(string) (string, error)) ServiceOption {
	return func(service *Service) {
		if generator != nil {
			service.newID = generator
		}
	}
}

// Service validates and executes direct in-process invocations.
type Service struct {
	registry *Registry
	store    InvocationStore
	now      func() time.Time
	newID    func(string) (string, error)

	activeMu sync.Mutex
	active   map[string]context.CancelFunc
}

func NewService(registry *Registry, store InvocationStore, options ...ServiceOption) *Service {
	if registry == nil {
		registry = NewRegistry()
	}
	if store == nil {
		store = NewMemoryStore()
	}
	service := &Service{
		registry: registry,
		store:    store,
		now:      func() time.Time { return time.Now().UTC() },
		newID:    randomID,
		active:   make(map[string]context.CancelFunc),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) Get(id string) (Invocation, error) {
	return s.store.Get(id)
}

func (s *Service) List() []Invocation {
	return s.store.List()
}

// Cancel signals a currently executing invocation. Final status is persisted
// by the Invoke call that owns execution.
func (s *Service) Cancel(id string) error {
	s.activeMu.Lock()
	cancel := s.active[id]
	s.activeMu.Unlock()
	if cancel == nil {
		if _, err := s.store.Get(id); err != nil {
			return err
		}
		return fmt.Errorf("%w: %s", ErrInvocationNotLive, id)
	}
	cancel()
	return nil
}

// Invoke executes one exact or aliased function reference synchronously.
func (s *Service) Invoke(ctx context.Context, request InvocationRequest) (Invocation, error) {
	invocation, err := s.newInvocation(request)
	if err != nil {
		return Invocation{}, err
	}

	resolved, function, resolveErr := s.registry.ResolveRef(request.Function)
	if resolveErr != nil {
		failure := Failure{
			Category: FailureFunctionNotFound,
			Code:     "function_not_found",
			Message:  resolveErr.Error(),
		}
		if errors.Is(resolveErr, ErrDigestPinMismatch) {
			failure.Category = FailureInvalidRequest
			failure.Code = "digest_pin_mismatch"
		}
		return s.rejectBeforeExecution(invocation, failure)
	}
	manifest := function.Manifest()
	invocation.Function = resolved
	invocation.SideEffects = manifest.SideEffects
	invocation.Context = request.Context.clone()
	if request.ProjectID != "" &&
		invocation.Context.ProjectID != "" &&
		request.ProjectID != invocation.Context.ProjectID {
		return s.rejectBeforeExecution(invocation, Failure{
			Category: FailureContextIncompatible,
			Code:     "project_context_mismatch",
			Message:  "request project_id and execution context project_id must match",
		})
	}

	canonicalInput, decodedInput, inputErr := normalizeInput(request.Input)
	if inputErr == nil {
		invocation.InputHash = digestBytes(canonicalInput)
		invocation.RedactedInput = redactInput(decodedInput, manifest.Redaction.SecretFields)
	} else {
		invocation.InputHash = digestBytes(request.Input)
	}
	if inputErr != nil {
		return s.rejectBeforeExecution(invocation, Failure{
			Category: FailureInputSchema,
			Code:     "invalid_json_input",
			Message:  inputErr.Error(),
		})
	}
	if err := manifest.InputSchema.Validate(decodedInput); err != nil {
		return s.rejectBeforeExecution(invocation, Failure{
			Category: FailureInputSchema,
			Code:     "input_schema_mismatch",
			Message:  err.Error(),
		})
	}
	if err := invocation.Context.validate(manifest); err != nil {
		failure := classifyFailure(err)
		return s.rejectBeforeExecution(invocation, failure)
	}

	timeoutMS := request.TimeoutMS
	if timeoutMS == 0 {
		timeoutMS = manifest.Timeout.DefaultMS
	}
	if timeoutMS < 0 || timeoutMS > manifest.Timeout.MaximumMS {
		return s.rejectBeforeExecution(invocation, Failure{
			Category: FailureInvalidRequest,
			Code:     "invalid_timeout",
			Message: fmt.Sprintf(
				"timeout_ms must be between 1 and %d", manifest.Timeout.MaximumMS,
			),
		})
	}

	if err := s.store.Create(invocation); err != nil {
		return Invocation{}, err
	}
	started := s.now()
	invocation.StartedAt = &started
	invocation.Status = InvocationRunning
	if err := s.store.Save(invocation); err != nil {
		return Invocation{}, err
	}

	executionCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	s.activeMu.Lock()
	s.active[invocation.ID] = cancel
	s.activeMu.Unlock()
	defer func() {
		cancel()
		s.activeMu.Lock()
		delete(s.active, invocation.ID)
		s.activeMu.Unlock()
	}()

	type outcome struct {
		result FunctionResult
		err    error
	}
	outcomes := make(chan outcome, 1)
	// Atomic functions are trusted compiled-in code and must cooperate with
	// cancellation. A deadline stops this service from waiting, but Go cannot
	// forcibly kill an uncooperative goroutine. External side-effecting runtimes
	// therefore require process isolation with independent lifecycle cleanup.
	go func() {
		result, executeErr := executeSafely(function, executionCtx, invocation.Context, canonicalInput)
		outcomes <- outcome{result: result, err: executeErr}
	}()

	var result FunctionResult
	var executeErr error
	select {
	case returned := <-outcomes:
		result, executeErr = returned.result, returned.err
		if contextErr := executionCtx.Err(); contextErr != nil {
			executeErr = contextErr
		}
	case <-executionCtx.Done():
		executeErr = executionCtx.Err()
	}

	finished := s.now()
	invocation.FinishedAt = &finished
	invocation.Usage = result.Usage
	invocation.Usage.DurationMS = max(int64(finished.Sub(started)/time.Millisecond), 0)
	invocation.Artifacts = append([]ArtifactReference(nil), result.Artifacts...)
	invocation.Output = append(json.RawMessage(nil), result.Output...)

	var outputErr error
	if len(result.Output) != 0 {
		outputErr = manifest.OutputSchema.ValidateJSON(result.Output)
		invocation.OutputSchemaValid = outputErr == nil
	} else {
		outputErr = fmt.Errorf("%w: function returned empty output", ErrInvalidJSON)
	}

	switch {
	case errors.Is(executeErr, context.DeadlineExceeded):
		invocation.Status = InvocationTimedOut
		invocation.Failure = &Failure{
			Category:  FailureTimeout,
			Code:      "function_timeout",
			Message:   fmt.Sprintf("function exceeded timeout of %dms", timeoutMS),
			Retryable: manifest.RetryAllowed(FailureTimeout),
		}
	case errors.Is(executeErr, context.Canceled):
		invocation.Status = InvocationCanceled
		invocation.Failure = &Failure{
			Category: FailureCanceled,
			Code:     "function_canceled",
			Message:  "function invocation was canceled",
		}
	case executeErr != nil:
		failure := classifyFailure(executeErr)
		failure.Retryable = failure.Retryable && manifest.RetryAllowed(failure.Category)
		invocation.Status = statusForFailure(failure.Category)
		invocation.Failure = &failure
	case outputErr != nil:
		invocation.Status = InvocationRejected
		invocation.Failure = &Failure{
			Category: FailureOutputSchema,
			Code:     "output_schema_mismatch",
			Message:  outputErr.Error(),
		}
	case resourceErr(manifest.Resources, invocation.Usage) != nil:
		invocation.Status = InvocationFailed
		failure := classifyFailure(resourceErr(manifest.Resources, invocation.Usage))
		invocation.Failure = &failure
	default:
		invocation.Status = InvocationSucceeded
	}

	if err := s.store.Save(invocation); err != nil {
		return Invocation{}, err
	}
	snapshot := cloneInvocation(invocation)
	if snapshot.Failure != nil {
		return snapshot, &InvocationError{InvocationID: snapshot.ID, Failure: *snapshot.Failure}
	}
	return snapshot, nil
}

func (s *Service) newInvocation(request InvocationRequest) (Invocation, error) {
	id, err := s.newID("fni")
	if err != nil {
		return Invocation{}, fmt.Errorf("generate invocation ID: %w", err)
	}
	traceID := request.TraceID
	if traceID == "" {
		traceID, err = s.newID("trace")
		if err != nil {
			return Invocation{}, fmt.Errorf("generate trace ID: %w", err)
		}
	}
	spanID, err := s.newID("span")
	if err != nil {
		return Invocation{}, fmt.Errorf("generate span ID: %w", err)
	}
	return Invocation{
		ID:        id,
		ProjectID: request.ProjectID,
		Function:  request.Function,
		Status:    InvocationAccepted,
		TraceID:   traceID,
		SpanID:    spanID,
		CreatedAt: s.now(),
		Context:   request.Context.clone(),
	}, nil
}

func (s *Service) rejectBeforeExecution(invocation Invocation, failure Failure) (Invocation, error) {
	finished := s.now()
	invocation.Status = InvocationRejected
	invocation.Failure = &failure
	invocation.FinishedAt = &finished
	if err := s.store.Create(invocation); err != nil {
		return Invocation{}, err
	}
	snapshot := cloneInvocation(invocation)
	return snapshot, &InvocationError{InvocationID: snapshot.ID, Failure: failure}
}

func executeSafely(
	function AtomicFunction,
	ctx context.Context,
	execution *ExecutionContext,
	input json.RawMessage,
) (result FunctionResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = FunctionResult{}
			err = &ClassifiedError{
				Category: FailureInternal,
				Code:     "function_panic",
				Message:  "function execution panicked",
			}
		}
	}()
	return function.Execute(ctx, execution.clone(), append(json.RawMessage(nil), input...))
}

func normalizeInput(raw json.RawMessage) (json.RawMessage, any, error) {
	value, err := decodeJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return canonical, value, nil
}

func redactInput(value any, secretFields []string) json.RawMessage {
	if len(secretFields) == 0 {
		encoded, _ := json.Marshal(value)
		return encoded
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	clone, err := decodeJSON(encoded)
	if err != nil {
		return nil
	}
	for _, path := range secretFields {
		redactPath(clone, strings.Split(strings.TrimPrefix(path, "$."), "."))
	}
	redacted, _ := json.Marshal(clone)
	return redacted
}

func redactPath(value any, path []string) {
	if len(path) == 0 {
		return
	}
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	if len(path) == 1 {
		if _, exists := object[path[0]]; exists {
			object[path[0]] = "[REDACTED]"
		}
		return
	}
	redactPath(object[path[0]], path[1:])
}

func resourceErr(policy ResourcePolicy, usage Usage) error {
	var exceeded []string
	if policy.MemoryBytes > 0 && usage.PeakRSSBytes > policy.MemoryBytes {
		exceeded = append(exceeded, "memory_bytes")
	}
	if policy.CPUMilliseconds > 0 && usage.CPUSeconds*1000 > float64(policy.CPUMilliseconds) {
		exceeded = append(exceeded, "cpu_ms")
	}
	if policy.NetworkBytes > 0 && usage.NetworkBytes > policy.NetworkBytes {
		exceeded = append(exceeded, "network_bytes")
	}
	if policy.ArtifactBytes > 0 && usage.ArtifactBytes > policy.ArtifactBytes {
		exceeded = append(exceeded, "artifact_bytes")
	}
	if len(exceeded) == 0 {
		return nil
	}
	return &ClassifiedError{
		Category: FailureResourceLimit,
		Code:     "resource_limit_exceeded",
		Message:  "function exceeded resource policy: " + strings.Join(exceeded, ", "),
		Details:  map[string]string{"limits": strings.Join(exceeded, ",")},
	}
}

func statusForFailure(category FailureCategory) InvocationStatus {
	switch category {
	case FailureInvalidRequest, FailureInputSchema, FailureOutputSchema,
		FailureContextIncompatible, FailureCapabilityMissing, FailureValidation:
		return InvocationRejected
	default:
		return InvocationFailed
	}
}

func cloneInvocation(invocation Invocation) Invocation {
	clone := invocation
	clone.RedactedInput = append(json.RawMessage(nil), invocation.RedactedInput...)
	clone.Output = append(json.RawMessage(nil), invocation.Output...)
	clone.Context = invocation.Context.clone()
	clone.Artifacts = append([]ArtifactReference(nil), invocation.Artifacts...)
	if invocation.StartedAt != nil {
		started := *invocation.StartedAt
		clone.StartedAt = &started
	}
	if invocation.FinishedAt != nil {
		finished := *invocation.FinishedAt
		clone.FinishedAt = &finished
	}
	if invocation.Failure != nil {
		failure := *invocation.Failure
		failure.Details = make(map[string]string, len(invocation.Failure.Details))
		for key, value := range invocation.Failure.Details {
			failure.Details[key] = value
		}
		clone.Failure = &failure
	}
	return clone
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func randomID(prefix string) (string, error) {
	size := 16
	switch prefix {
	case "span":
		size = 8
	}
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	encoded := hex.EncodeToString(value)
	if prefix == "trace" || prefix == "span" {
		return encoded, nil
	}
	return prefix + "_" + encoded, nil
}
