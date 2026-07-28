package function

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestServiceInvokesEchoWithPinnedMetadata(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore()
	service := NewService(registry, store)

	invocation, err := service.Invoke(context.Background(), InvocationRequest{
		ProjectID: "prj_test",
		Function:  FunctionRef{Name: "system.echo", Version: AliasStable},
		Input:     json.RawMessage(`{"b":2,"a":1}`),
		TraceID:   "0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("invoke echo: %v", err)
	}
	if invocation.Status != InvocationSucceeded {
		t.Fatalf("status = %s", invocation.Status)
	}
	if invocation.Function.Version != "1" || !strings.HasPrefix(invocation.Function.Digest, "sha256:") {
		t.Fatalf("function was not exactly pinned: %#v", invocation.Function)
	}
	if string(invocation.Output) != `{"a":1,"b":2}` {
		t.Fatalf("output = %s", invocation.Output)
	}
	if !invocation.OutputSchemaValid {
		t.Fatal("output was not marked schema valid")
	}
	if len(invocation.TraceID) != 32 || len(invocation.SpanID) != 16 {
		t.Fatalf("invalid trace IDs: trace=%q span=%q", invocation.TraceID, invocation.SpanID)
	}
	if invocation.TraceID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("caller trace ID was not propagated: %q", invocation.TraceID)
	}
	if invocation.StartedAt == nil || invocation.FinishedAt == nil {
		t.Fatal("execution timestamps were not recorded")
	}

	invocation.Output[0] = '['
	stored, err := service.Get(invocation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored.Output) != `{"a":1,"b":2}` {
		t.Fatal("store snapshot was mutated through returned invocation")
	}
}

func TestServiceRejectsInputBeforeExecution(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	manifest := baseTestManifest("test.input", "1")
	manifest.InputSchema = Schema{
		Type:     TypeObject,
		Required: []string{"count"},
		Properties: map[string]Schema{
			"count": {Type: TypeInteger, Minimum: Number(1)},
		},
		AdditionalProperties: Bool(false),
	}
	function, err := NewAtomicFunction(manifest, func(
		context.Context, *ExecutionContext, json.RawMessage,
	) (FunctionResult, error) {
		calls.Add(1)
		return FunctionResult{Output: json.RawMessage(`null`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}
	service := NewService(registry, NewMemoryStore())

	invocation, invokeErr := service.Invoke(context.Background(), InvocationRequest{
		Function: FunctionRef{Name: "test.input", Version: "1"},
		Input:    json.RawMessage(`{"count":0,"extra":true}`),
	})
	assertInvocationFailure(t, invocation, invokeErr, InvocationRejected, FailureInputSchema)
	if calls.Load() != 0 {
		t.Fatalf("executor was called %d times", calls.Load())
	}
	stored, err := service.Get(invocation.ID)
	if err != nil || stored.Status != InvocationRejected {
		t.Fatalf("preflight rejection was not stored: %#v, %v", stored, err)
	}
}

func TestServiceRejectsMalformedAndMismatchedOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output json.RawMessage
	}{
		{"malformed", json.RawMessage(`{`)},
		{"wrong type", json.RawMessage(`"not an object"`)},
		{"empty", nil},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := baseTestManifest("test.output_"+strings.ReplaceAll(test.name, " ", "_"), "1")
			manifest.OutputSchema = Schema{
				Type:     TypeObject,
				Required: []string{"ok"},
				Properties: map[string]Schema{
					"ok": {Type: TypeBoolean},
				},
				AdditionalProperties: Bool(false),
			}
			function, err := NewAtomicFunction(manifest, func(
				context.Context, *ExecutionContext, json.RawMessage,
			) (FunctionResult, error) {
				return FunctionResult{Output: test.output}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			registry := NewRegistry()
			if err := registry.Register(function); err != nil {
				t.Fatal(err)
			}
			invocation, invokeErr := NewService(registry, nil).Invoke(
				context.Background(),
				InvocationRequest{
					Function: FunctionRef{Name: manifest.Name, Version: "1"},
					Input:    json.RawMessage(`null`),
				},
			)
			assertInvocationFailure(t, invocation, invokeErr, InvocationRejected, FailureOutputSchema)
			if invocation.OutputSchemaValid {
				t.Fatal("invalid output marked valid")
			}
		})
	}
}

func TestServiceClassifiesErrorsAndConstrainsRetry(t *testing.T) {
	t.Parallel()
	for _, sideEffects := range []SideEffectClass{SideEffectIdempotent, SideEffectNonIdempotent} {
		sideEffects := sideEffects
		t.Run(string(sideEffects), func(t *testing.T) {
			t.Parallel()
			manifest := baseTestManifest("test.retry_"+string(sideEffects), "1")
			manifest.SideEffects = sideEffects
			manifest.Retry.AllowedFailures = []FailureCategory{FailureTransientNetwork}
			function, err := NewAtomicFunction(manifest, func(
				context.Context, *ExecutionContext, json.RawMessage,
			) (FunctionResult, error) {
				return FunctionResult{Output: json.RawMessage(`null`)}, &ClassifiedError{
					Category:  FailureTransientNetwork,
					Code:      "connection_reset",
					Message:   "connection reset by peer",
					Retryable: true,
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			registry := NewRegistry()
			if err := registry.Register(function); err != nil {
				t.Fatal(err)
			}
			invocation, invokeErr := NewService(registry, nil).Invoke(
				context.Background(),
				InvocationRequest{
					Function: FunctionRef{Name: manifest.Name, Version: "1"},
					Input:    json.RawMessage(`null`),
				},
			)
			assertInvocationFailure(t, invocation, invokeErr, InvocationFailed, FailureTransientNetwork)
			wantRetry := sideEffects == SideEffectIdempotent
			if invocation.Failure.Retryable != wantRetry {
				t.Fatalf("retryable = %v, want %v", invocation.Failure.Retryable, wantRetry)
			}
			if !invocation.OutputSchemaValid {
				t.Fatal("result output should still be validated when execution returns a classified error")
			}
		})
	}
}

func TestServiceEnforcesTimeoutEvenWhenFunctionIgnoresContext(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	manifest := baseTestManifest("test.timeout", "1")
	manifest.Timeout = TimeoutPolicy{DefaultMS: 15, MaximumMS: 50}
	manifest.Retry.AllowedFailures = []FailureCategory{FailureTimeout}
	function, err := NewAtomicFunction(manifest, func(
		context.Context, *ExecutionContext, json.RawMessage,
	) (FunctionResult, error) {
		<-release
		return FunctionResult{Output: json.RawMessage(`null`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	invocation, invokeErr := NewService(registry, nil).Invoke(
		context.Background(),
		InvocationRequest{
			Function: FunctionRef{Name: manifest.Name, Version: "1"},
			Input:    json.RawMessage(`null`),
		},
	)
	elapsed := time.Since(start)
	close(release)
	assertInvocationFailure(t, invocation, invokeErr, InvocationTimedOut, FailureTimeout)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("timeout was not enforced promptly: %s", elapsed)
	}
	if !invocation.Failure.Retryable {
		t.Fatal("pure timeout allowlisted by manifest should be retryable")
	}
}

func TestServiceCancellationPersistsTerminalStatus(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	manifest := baseTestManifest("test.cancel", "1")
	function, err := NewAtomicFunction(manifest, func(
		ctx context.Context, _ *ExecutionContext, _ json.RawMessage,
	) (FunctionResult, error) {
		close(started)
		<-ctx.Done()
		return FunctionResult{}, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}
	service := NewService(registry, nil, WithIDGenerator(func(prefix string) (string, error) {
		return prefix + "-fixed", nil
	}))
	type response struct {
		invocation Invocation
		err        error
	}
	done := make(chan response, 1)
	go func() {
		invocation, invokeErr := service.Invoke(context.Background(), InvocationRequest{
			Function: FunctionRef{Name: manifest.Name, Version: "1"},
			Input:    json.RawMessage(`null`),
		})
		done <- response{invocation: invocation, err: invokeErr}
	}()
	<-started
	if err := service.Cancel("fni-fixed"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	result := <-done
	assertInvocationFailure(t, result.invocation, result.err, InvocationCanceled, FailureCanceled)
	stored, err := service.Get("fni-fixed")
	if err != nil || stored.Status != InvocationCanceled {
		t.Fatalf("canceled result not persisted: %#v, %v", stored, err)
	}
}

func TestServiceChecksExecutionContextAndCapabilities(t *testing.T) {
	t.Parallel()
	manifest := baseTestManifest("test.browser", "1")
	manifest.ExecutionContext = ExecutionContextBrowserOrSession
	manifest.Capabilities = []string{"chromium"}
	function, err := NewAtomicFunction(manifest, func(
		context.Context, *ExecutionContext, json.RawMessage,
	) (FunctionResult, error) {
		return FunctionResult{Output: json.RawMessage(`null`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}
	service := NewService(registry, nil)

	invocation, invokeErr := service.Invoke(context.Background(), InvocationRequest{
		Function: FunctionRef{Name: manifest.Name, Version: "1"},
		Input:    json.RawMessage(`null`),
	})
	assertInvocationFailure(t, invocation, invokeErr, InvocationRejected, FailureContextIncompatible)

	invocation, invokeErr = service.Invoke(context.Background(), InvocationRequest{
		Function: FunctionRef{Name: manifest.Name, Version: "1"},
		Context:  &ExecutionContext{EphemeralBrowser: true},
		Input:    json.RawMessage(`null`),
	})
	assertInvocationFailure(t, invocation, invokeErr, InvocationRejected, FailureCapabilityMissing)

	invocation, invokeErr = service.Invoke(context.Background(), InvocationRequest{
		Function: FunctionRef{Name: manifest.Name, Version: "1"},
		Context: &ExecutionContext{
			SessionID: "ses_test", Capabilities: []string{"chromium"},
		},
		Input: json.RawMessage(`null`),
	})
	if invokeErr != nil || invocation.Status != InvocationSucceeded {
		t.Fatalf("valid session context rejected: %#v, %v", invocation, invokeErr)
	}
}

func TestServiceRejectsMismatchedProjectContextBeforeExecution(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	manifest := baseTestManifest("test.project_context", "1")
	function, err := NewAtomicFunction(manifest, func(
		context.Context, *ExecutionContext, json.RawMessage,
	) (FunctionResult, error) {
		calls.Add(1)
		return FunctionResult{Output: json.RawMessage(`null`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}
	service := NewService(registry, nil)

	invocation, invokeErr := service.Invoke(context.Background(), InvocationRequest{
		ProjectID: "prj_request",
		Function:  FunctionRef{Name: manifest.Name, Version: "1"},
		Context:   &ExecutionContext{ProjectID: "prj_other"},
		Input:     json.RawMessage(`null`),
	})
	assertInvocationFailure(
		t,
		invocation,
		invokeErr,
		InvocationRejected,
		FailureContextIncompatible,
	)
	if invocation.Failure.Code != "project_context_mismatch" {
		t.Fatalf("failure code = %q", invocation.Failure.Code)
	}
	if calls.Load() != 0 {
		t.Fatalf("executor was called %d times for a cross-project context", calls.Load())
	}
	stored, err := service.Get(invocation.ID)
	if err != nil {
		t.Fatalf("get rejected invocation: %v", err)
	}
	if stored.Failure == nil || stored.Failure.Code != "project_context_mismatch" {
		t.Fatalf("stored failure = %#v", stored.Failure)
	}
}

func TestServiceHashesAndRedactsInput(t *testing.T) {
	t.Parallel()
	manifest := baseTestManifest("test.redact", "1")
	manifest.Redaction.SecretFields = []string{"credentials.token", "password"}
	function, err := NewAtomicFunction(manifest, func(
		context.Context, *ExecutionContext, json.RawMessage,
	) (FunctionResult, error) {
		return FunctionResult{Output: json.RawMessage(`null`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`{"password":"secret","credentials":{"token":"abc","user":"d"},"safe":true}`)
	invocation, invokeErr := NewService(registry, nil).Invoke(context.Background(), InvocationRequest{
		Function: FunctionRef{Name: manifest.Name, Version: "1"},
		Input:    input,
	})
	if invokeErr != nil {
		t.Fatal(invokeErr)
	}
	if !strings.HasPrefix(invocation.InputHash, "sha256:") || len(invocation.InputHash) != 71 {
		t.Fatalf("invalid input hash %q", invocation.InputHash)
	}
	redacted := string(invocation.RedactedInput)
	if strings.Contains(redacted, "secret") || strings.Contains(redacted, `"abc"`) {
		t.Fatalf("secret leaked in redacted input: %s", redacted)
	}
	if !strings.Contains(redacted, `"user":"d"`) || !strings.Contains(redacted, `"safe":true`) {
		t.Fatalf("safe fields were lost: %s", redacted)
	}
}

type adversarialPanicValue struct {
	secret    string
	formatted *atomic.Bool
}

func (v *adversarialPanicValue) String() string {
	v.formatted.Store(true)
	return v.secret
}

func TestServicePanicIsOpaqueForAdversarialValues(t *testing.T) {
	t.Parallel()
	const secret = "secret-that-must-never-cross-the-function-boundary"
	var formatted atomic.Bool
	manifest := baseTestManifest("test.opaque_panic", "1")
	function, err := NewAtomicFunction(manifest, func(
		context.Context, *ExecutionContext, json.RawMessage,
	) (FunctionResult, error) {
		panic(&adversarialPanicValue{secret: secret, formatted: &formatted})
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}

	invocation, invokeErr := NewService(registry, nil).Invoke(
		context.Background(),
		InvocationRequest{
			Function: FunctionRef{Name: manifest.Name, Version: "1"},
			Input:    json.RawMessage(`null`),
		},
	)
	assertInvocationFailure(t, invocation, invokeErr, InvocationFailed, FailureInternal)
	if formatted.Load() {
		t.Fatal("panic value was formatted while being classified")
	}
	if invocation.Failure.Code != "function_panic" {
		t.Fatalf("failure code = %q", invocation.Failure.Code)
	}
	if len(invocation.Failure.Details) != 0 {
		t.Fatalf("panic details must be opaque, got %#v", invocation.Failure.Details)
	}
	encoded, err := json.Marshal(invocation)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(invokeErr.Error(), secret) {
		t.Fatalf("panic secret escaped classification: invocation=%s error=%v", encoded, invokeErr)
	}
}

func TestServiceClassifiesResourcePolicyAndPanic(t *testing.T) {
	t.Parallel()
	t.Run("resource", func(t *testing.T) {
		t.Parallel()
		manifest := baseTestManifest("test.resource", "1")
		manifest.Resources.NetworkBytes = 10
		function, err := NewAtomicFunction(manifest, func(
			context.Context, *ExecutionContext, json.RawMessage,
		) (FunctionResult, error) {
			return FunctionResult{
				Output: json.RawMessage(`null`),
				Usage:  Usage{NetworkBytes: 11},
			}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		registry := NewRegistry()
		if err := registry.Register(function); err != nil {
			t.Fatal(err)
		}
		invocation, invokeErr := NewService(registry, nil).Invoke(context.Background(), InvocationRequest{
			Function: FunctionRef{Name: manifest.Name, Version: "1"},
			Input:    json.RawMessage(`null`),
		})
		assertInvocationFailure(t, invocation, invokeErr, InvocationFailed, FailureResourceLimit)
	})

	t.Run("panic", func(t *testing.T) {
		t.Parallel()
		manifest := baseTestManifest("test.panic", "1")
		function, err := NewAtomicFunction(manifest, func(
			context.Context, *ExecutionContext, json.RawMessage,
		) (FunctionResult, error) {
			panic("boom")
		})
		if err != nil {
			t.Fatal(err)
		}
		registry := NewRegistry()
		if err := registry.Register(function); err != nil {
			t.Fatal(err)
		}
		invocation, invokeErr := NewService(registry, nil).Invoke(context.Background(), InvocationRequest{
			Function: FunctionRef{Name: manifest.Name, Version: "1"},
			Input:    json.RawMessage(`null`),
		})
		assertInvocationFailure(t, invocation, invokeErr, InvocationFailed, FailureInternal)
	})
}

func TestMemoryStoreCreateSaveAndCopies(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	invocation := Invocation{
		ID:            "fni_store",
		Status:        InvocationAccepted,
		CreatedAt:     time.Unix(1, 0),
		RedactedInput: json.RawMessage(`{"secret":"[REDACTED]"}`),
	}
	if err := store.Create(invocation); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(invocation); !errors.Is(err, ErrInvocationExists) {
		t.Fatalf("expected duplicate create error, got %v", err)
	}
	invocation.Status = InvocationSucceeded
	if err := store.Save(invocation); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(invocation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != InvocationSucceeded {
		t.Fatalf("status = %s", got.Status)
	}
	got.RedactedInput[0] = '['
	again, _ := store.Get(invocation.ID)
	if string(again.RedactedInput) != `{"secret":"[REDACTED]"}` {
		t.Fatal("stored JSON mutated through Get result")
	}
	if err := store.Save(Invocation{ID: "missing"}); !errors.Is(err, ErrInvocationNotFound) {
		t.Fatalf("expected not found save error, got %v", err)
	}
}

func assertInvocationFailure(
	t *testing.T,
	invocation Invocation,
	err error,
	status InvocationStatus,
	category FailureCategory,
) {
	t.Helper()
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("expected InvocationError, got %v", err)
	}
	if invocation.Status != status {
		t.Fatalf("status = %s, want %s", invocation.Status, status)
	}
	if invocation.Failure == nil || invocation.Failure.Category != category {
		t.Fatalf("failure = %#v, want category %s", invocation.Failure, category)
	}
	if invocationErr.InvocationID != invocation.ID {
		t.Fatalf("error invocation ID %q != %q", invocationErr.InvocationID, invocation.ID)
	}
}
