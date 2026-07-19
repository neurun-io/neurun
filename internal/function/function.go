package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// FailureCategory is stable machine-readable failure metadata persisted with
// every unsuccessful invocation.
type FailureCategory string

const (
	FailureInvalidRequest      FailureCategory = "invalid_request"
	FailureFunctionNotFound    FailureCategory = "function_not_found"
	FailureInputSchema         FailureCategory = "input_schema_mismatch"
	FailureOutputSchema        FailureCategory = "output_schema_mismatch"
	FailureContextIncompatible FailureCategory = "execution_context_incompatible"
	FailureCapabilityMissing   FailureCategory = "capability_missing"
	FailureValidation          FailureCategory = "validation_failed"
	FailureTimeout             FailureCategory = "timeout"
	FailureCanceled            FailureCategory = "canceled"
	FailureTransientNetwork    FailureCategory = "transient_network"
	FailureBrowserCrash        FailureCategory = "browser_crash"
	FailureAgentLost           FailureCategory = "agent_lost"
	FailureResourceLimit       FailureCategory = "resource_limit_exceeded"
	FailureInternal            FailureCategory = "internal_error"
	FailureExecution           FailureCategory = "execution_failed"
)

// ClassifiedError lets a built-in return an explainable failure without
// exposing implementation errors through the public API.
type ClassifiedError struct {
	Category  FailureCategory   `json:"category"`
	Code      string            `json:"code,omitempty"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details,omitempty"`
	Cause     error             `json:"-"`
}

func (e *ClassifiedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Message
}

func (e *ClassifiedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewClassifiedError constructs an execution error with stable public fields.
func NewClassifiedError(category FailureCategory, code, message string, retryable bool) *ClassifiedError {
	return &ClassifiedError{
		Category: category, Code: code, Message: message, Retryable: retryable,
	}
}

// Failure is the persisted, serializable form of a classified error.
type Failure struct {
	Category  FailureCategory   `json:"category"`
	Code      string            `json:"code,omitempty"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details,omitempty"`
}

func classifyFailure(err error) Failure {
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		details := make(map[string]string, len(classified.Details))
		for key, value := range classified.Details {
			details[key] = value
		}
		return Failure{
			Category:  classified.Category,
			Code:      classified.Code,
			Message:   classified.Message,
			Retryable: classified.Retryable,
			Details:   details,
		}
	}
	return Failure{
		Category: FailureExecution,
		Code:     "function_execution_failed",
		Message:  "function execution failed",
	}
}

// Usage is the per-invocation resource and timing ledger. Unavailable metrics
// remain zero in this in-process MVP store.
type Usage struct {
	DurationMS    int64   `json:"duration_ms"`
	CPUSeconds    float64 `json:"cpu_seconds,omitempty"`
	PeakRSSBytes  int64   `json:"peak_rss_bytes,omitempty"`
	NetworkBytes  int64   `json:"network_bytes,omitempty"`
	ArtifactBytes int64   `json:"artifact_bytes,omitempty"`
}

type ArtifactReference struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

// FunctionResult is the schema-validated result of one AtomicFunction.
type FunctionResult struct {
	Output    json.RawMessage     `json:"output"`
	Usage     Usage               `json:"usage,omitempty"`
	Artifacts []ArtifactReference `json:"artifacts,omitempty"`
}

// ExecutionContext is the bounded server-owned context supplied to a
// function. Atomic functions do not receive database, queue, or agent handles.
type ExecutionContext struct {
	ProjectID        string   `json:"project_id,omitempty"`
	JobID            string   `json:"job_id,omitempty"`
	AttemptID        string   `json:"attempt_id,omitempty"`
	SessionID        string   `json:"session_id,omitempty"`
	WorkflowStepID   string   `json:"workflow_step_id,omitempty"`
	EphemeralHTTP    bool     `json:"ephemeral_http,omitempty"`
	EphemeralBrowser bool     `json:"ephemeral_browser,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
}

func (e *ExecutionContext) clone() *ExecutionContext {
	if e == nil {
		return &ExecutionContext{}
	}
	clone := *e
	clone.Capabilities = append([]string(nil), e.Capabilities...)
	sort.Strings(clone.Capabilities)
	return &clone
}

func (e *ExecutionContext) validate(manifest Manifest) error {
	if e == nil {
		e = &ExecutionContext{}
	}
	switch manifest.ExecutionContext {
	case ExecutionContextNone:
		// Context metadata may still carry project and workflow correlation.
	case ExecutionContextHTTPAttempt:
		if e.AttemptID == "" && !e.EphemeralHTTP {
			return NewClassifiedError(
				FailureContextIncompatible,
				"http_attempt_required",
				"function requires an HTTP attempt or context.ephemeral_http",
				false,
			)
		}
	case ExecutionContextBrowserAttempt:
		if e.AttemptID == "" && !e.EphemeralBrowser {
			return NewClassifiedError(
				FailureContextIncompatible,
				"browser_attempt_required",
				"function requires a browser attempt or context.ephemeral_browser",
				false,
			)
		}
	case ExecutionContextExistingSession:
		if e.SessionID == "" {
			return NewClassifiedError(
				FailureContextIncompatible,
				"session_required",
				"function requires an existing session",
				false,
			)
		}
	case ExecutionContextBrowserOrSession:
		if e.SessionID == "" && e.AttemptID == "" && !e.EphemeralBrowser {
			return NewClassifiedError(
				FailureContextIncompatible,
				"browser_or_session_required",
				"function requires a browser attempt, an existing session, or context.ephemeral_browser",
				false,
			)
		}
	default:
		return NewClassifiedError(
			FailureInternal, "invalid_manifest_context", "function has an invalid execution context", false,
		)
	}

	available := make(map[string]struct{}, len(e.Capabilities))
	for _, capability := range e.Capabilities {
		available[capability] = struct{}{}
	}
	var missing []string
	for _, capability := range manifest.Capabilities {
		if _, ok := available[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	if len(missing) != 0 {
		return &ClassifiedError{
			Category: FailureCapabilityMissing,
			Code:     "required_capability_missing",
			Message:  "execution context is missing required capabilities: " + strings.Join(missing, ", "),
			Details:  map[string]string{"missing": strings.Join(missing, ",")},
		}
	}
	return nil
}

// AtomicFunction is the server-internal immutable execution boundary.
type AtomicFunction interface {
	Manifest() Manifest
	Execute(
		ctx context.Context,
		execution *ExecutionContext,
		input json.RawMessage,
	) (FunctionResult, error)
}

// Executor adapts a function to the AtomicFunction execution method.
type Executor func(context.Context, *ExecutionContext, json.RawMessage) (FunctionResult, error)

type atomicFunction struct {
	manifest Manifest
	execute  Executor
}

// NewAtomicFunction seals a manifest and binds it to an executor.
func NewAtomicFunction(manifest Manifest, execute Executor) (AtomicFunction, error) {
	if execute == nil {
		return nil, errors.New("atomic function executor is required")
	}
	normalized, err := NormalizeManifest(manifest)
	if err != nil {
		return nil, err
	}
	return &atomicFunction{manifest: normalized, execute: execute}, nil
}

func (f *atomicFunction) Manifest() Manifest {
	return f.manifest.Clone()
}

func (f *atomicFunction) Execute(
	ctx context.Context,
	execution *ExecutionContext,
	input json.RawMessage,
) (FunctionResult, error) {
	return f.execute(ctx, execution, append(json.RawMessage(nil), input...))
}
