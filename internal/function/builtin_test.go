package function

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestBuiltinBundleContainsSealedFoundationFunctions(t *testing.T) {
	t.Parallel()
	bundle, err := BuiltinBundle()
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("bundle invalid: %v", err)
	}
	if len(bundle.Manifests) != 2 {
		t.Fatalf("got %d manifests, want 2", len(bundle.Manifests))
	}
	names := map[string]bool{}
	for _, manifest := range bundle.Manifests {
		names[manifest.Name+"@"+manifest.Version] = true
		if manifest.Digest == "" {
			t.Fatalf("%s has no digest", manifest.Name)
		}
	}
	for _, required := range []string{"system.echo@1", "validate.output@1"} {
		if !names[required] {
			t.Errorf("bundle is missing %s", required)
		}
	}
}

func TestPublishedBuiltinBundleMatchesRuntime(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../contracts/functions/bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	var published ManifestBundle
	if err := json.Unmarshal(raw, &published); err != nil {
		t.Fatalf("decode published bundle: %v", err)
	}
	actual, err := BuiltinBundle()
	if err != nil {
		t.Fatal(err)
	}
	actualJSON, err := actual.MarshalNormalized()
	if err != nil {
		t.Fatal(err)
	}
	publishedJSON, err := published.MarshalNormalized()
	if err != nil {
		t.Fatalf("normalize published bundle: %v\nactual: %s", err, actualJSON)
	}
	if !bytes.Equal(publishedJSON, actualJSON) {
		t.Fatalf("published bundle differs from runtime\npublished: %s\nactual: %s", publishedJSON, actualJSON)
	}
}

func TestValidateOutputBuiltinAcceptsValidData(t *testing.T) {
	t.Parallel()
	service := builtinService(t)
	invocation, err := service.Invoke(context.Background(), InvocationRequest{
		Function: FunctionRef{Name: "validate.output", Version: "1"},
		Input: json.RawMessage(`{
			"value":[{"id":1},{"id":2}],
			"schema":{"type":"array","items":{"type":"object"}},
			"rules":{"min_records":2,"max_records":3,"required_fields":["id"],"non_empty":true}
		}`),
	})
	if err != nil {
		t.Fatalf("valid data rejected: %v (%s)", err, invocation.Output)
	}
	var output struct {
		Valid      bool     `json:"valid"`
		Violations []string `json:"violations"`
	}
	if err := json.Unmarshal(invocation.Output, &output); err != nil {
		t.Fatal(err)
	}
	if !output.Valid || len(output.Violations) != 0 {
		t.Fatalf("unexpected validation output: %#v", output)
	}
}

func TestValidateOutputBuiltinReturnsEvidenceOnRejection(t *testing.T) {
	t.Parallel()
	service := builtinService(t)
	invocation, err := service.Invoke(context.Background(), InvocationRequest{
		Function: FunctionRef{Name: "validate.output", Version: AliasStable},
		Input: json.RawMessage(`{
			"value_from":[{"id":1},{}],
			"rules":{"min_records":3,"required_fields":["id"]}
		}`),
	})
	assertInvocationFailure(t, invocation, err, InvocationRejected, FailureValidation)
	if !invocation.OutputSchemaValid {
		t.Fatal("validation evidence output did not satisfy its own schema")
	}
	var output struct {
		Valid      bool     `json:"valid"`
		Violations []string `json:"violations"`
	}
	if unmarshalErr := json.Unmarshal(invocation.Output, &output); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if output.Valid || len(output.Violations) != 2 {
		t.Fatalf("unexpected rejection evidence: %#v", output)
	}
}

func TestValidateOutputBuiltinRejectsBadRuleAndSchemaRequests(t *testing.T) {
	t.Parallel()
	service := builtinService(t)
	tests := []json.RawMessage{
		json.RawMessage(`{"rules":{"min_records":1}}`),
		json.RawMessage(`{"value":1,"schema":{"type":"wat"}}`),
		json.RawMessage(`{"value":1,"rules":{"unknown":true}}`),
	}
	for _, input := range tests {
		invocation, err := service.Invoke(context.Background(), InvocationRequest{
			Function: FunctionRef{Name: "validate.output", Version: "1"},
			Input:    input,
		})
		var invocationErr *InvocationError
		if !errors.As(err, &invocationErr) || invocation.Status != InvocationRejected {
			t.Errorf("input %s: expected rejection, got %#v, %v", input, invocation, err)
		}
	}
}

func builtinService(t *testing.T) *Service {
	t.Helper()
	registry := NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatal(err)
	}
	return NewService(registry, NewMemoryStore())
}
