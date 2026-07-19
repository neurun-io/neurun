package function

import (
	"context"
	"encoding/json"
	"fmt"
)

const BuiltinBundleVersion = "1"

// Builtins returns the minimal diagnostic and output-validation catalog used
// by the active foundation.
func Builtins() ([]AtomicFunction, error) {
	echo, err := NewAtomicFunction(echoManifest(), executeEcho)
	if err != nil {
		return nil, err
	}
	validate, err := NewAtomicFunction(validateOutputManifest(), executeValidateOutput)
	if err != nil {
		return nil, err
	}
	return []AtomicFunction{echo, validate}, nil
}

func RegisterBuiltins(registry *Registry) error {
	functions, err := Builtins()
	if err != nil {
		return err
	}
	return registry.RegisterAll(functions...)
}

func BuiltinBundle() (ManifestBundle, error) {
	functions, err := Builtins()
	if err != nil {
		return ManifestBundle{}, err
	}
	manifests := make([]Manifest, len(functions))
	for i, function := range functions {
		manifests[i] = function.Manifest()
	}
	return NormalizeBundle(BuiltinBundleVersion, manifests)
}

func echoManifest() Manifest {
	return Manifest{
		Name:             "system.echo",
		Version:          "1",
		Category:         "system",
		Description:      "Returns the supplied JSON value unchanged.",
		ExecutionContext: ExecutionContextNone,
		SideEffects:      SideEffectPure,
		Timeout: TimeoutPolicy{
			DefaultMS: 1000,
			MaximumMS: 5000,
		},
		InputSchema:  Schema{},
		OutputSchema: Schema{},
		Retry: RetryPolicy{AllowedFailures: []FailureCategory{
			FailureAgentLost,
		}},
		Telemetry: TelemetryPolicy{Dimensions: []string{"duration_ms"}},
	}
}

func executeEcho(
	ctx context.Context,
	_ *ExecutionContext,
	input json.RawMessage,
) (FunctionResult, error) {
	if err := ctx.Err(); err != nil {
		return FunctionResult{}, err
	}
	return FunctionResult{Output: append(json.RawMessage(nil), input...)}, nil
}

func validateOutputManifest() Manifest {
	return Manifest{
		Name:             "validate.output",
		Version:          "1",
		Category:         "validate",
		Description:      "Validates a value against a schema and bounded data-quality rules.",
		ExecutionContext: ExecutionContextNone,
		SideEffects:      SideEffectPure,
		Timeout: TimeoutPolicy{
			DefaultMS: 1000,
			MaximumMS: 5000,
		},
		InputSchema: Schema{
			Type: TypeObject,
			Properties: map[string]Schema{
				"value":      {},
				"value_from": {},
				"schema": {
					Type:                 TypeObject,
					AdditionalProperties: Bool(true),
				},
				"rules": {
					Type: TypeObject,
					Properties: map[string]Schema{
						"required_fields": {
							Type:  TypeArray,
							Items: &Schema{Type: TypeString, MinLength: Int(1)},
						},
						"min_records": {Type: TypeInteger, Minimum: Number(0)},
						"max_records": {Type: TypeInteger, Minimum: Number(0)},
						"non_empty":   {Type: TypeBoolean},
					},
					AdditionalProperties: Bool(false),
				},
			},
			AdditionalProperties: Bool(false),
		},
		OutputSchema: Schema{
			Type:     TypeObject,
			Required: []string{"valid", "violations"},
			Properties: map[string]Schema{
				"valid": {
					Type: TypeBoolean,
				},
				"violations": {
					Type:  TypeArray,
					Items: &Schema{Type: TypeString},
				},
			},
			AdditionalProperties: Bool(false),
		},
		Telemetry: TelemetryPolicy{Dimensions: []string{"duration_ms"}},
	}
}

func executeValidateOutput(
	ctx context.Context,
	_ *ExecutionContext,
	input json.RawMessage,
) (FunctionResult, error) {
	if err := ctx.Err(); err != nil {
		return FunctionResult{}, err
	}
	decoded, err := decodeJSON(input)
	if err != nil {
		return FunctionResult{}, NewClassifiedError(
			FailureInvalidRequest, "invalid_validation_input", err.Error(), false,
		)
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return FunctionResult{}, NewClassifiedError(
			FailureInvalidRequest, "invalid_validation_input", "validation input must be an object", false,
		)
	}
	value, exists := object["value"]
	if !exists {
		value, exists = object["value_from"]
	}
	if !exists {
		return FunctionResult{}, NewClassifiedError(
			FailureInvalidRequest,
			"value_required",
			"either value or value_from is required",
			false,
		)
	}

	var violations []string
	if rawSchema, schemaExists := object["schema"]; schemaExists {
		encodedSchema, marshalErr := json.Marshal(rawSchema)
		if marshalErr != nil {
			return FunctionResult{}, NewClassifiedError(
				FailureInvalidRequest, "invalid_schema", marshalErr.Error(), false,
			)
		}
		schema, schemaErr := DecodeSchema(encodedSchema)
		if schemaErr != nil {
			return FunctionResult{}, NewClassifiedError(
				FailureInvalidRequest, "invalid_schema", schemaErr.Error(), false,
			)
		}
		if validationErr := schema.Validate(value); validationErr != nil {
			violations = append(violations, "schema: "+validationErr.Error())
		}
	}

	if rawRules, rulesExist := object["rules"]; rulesExist {
		rules, ok := rawRules.(map[string]any)
		if !ok {
			return FunctionResult{}, NewClassifiedError(
				FailureInvalidRequest, "invalid_rules", "rules must be an object", false,
			)
		}
		violations = append(violations, validateRules(value, rules)...)
	}

	if violations == nil {
		violations = []string{}
	}
	output, marshalErr := json.Marshal(map[string]any{
		"valid":      len(violations) == 0,
		"violations": violations,
	})
	if marshalErr != nil {
		return FunctionResult{}, fmt.Errorf("encode validation result: %w", marshalErr)
	}
	result := FunctionResult{Output: output}
	if len(violations) != 0 {
		return result, &ClassifiedError{
			Category: FailureValidation,
			Code:     "output_validation_failed",
			Message:  fmt.Sprintf("output failed %d validation rule(s)", len(violations)),
			Details:  map[string]string{"violations": fmt.Sprintf("%d", len(violations))},
		}
	}
	return result, nil
}

func validateRules(value any, rules map[string]any) []string {
	var violations []string
	records, recordCount := recordValues(value)

	if minimum, exists := integerRule(rules, "min_records"); exists && recordCount < minimum {
		violations = append(violations, fmt.Sprintf(
			"min_records: got %d records, need at least %d", recordCount, minimum,
		))
	}
	if maximum, exists := integerRule(rules, "max_records"); exists && recordCount > maximum {
		violations = append(violations, fmt.Sprintf(
			"max_records: got %d records, allow at most %d", recordCount, maximum,
		))
	}
	if nonEmpty, _ := rules["non_empty"].(bool); nonEmpty && isEmptyJSONValue(value) {
		violations = append(violations, "non_empty: value is empty")
	}
	if required, exists := rules["required_fields"].([]any); exists {
		for recordIndex, record := range records {
			object, ok := record.(map[string]any)
			if !ok {
				violations = append(violations, fmt.Sprintf(
					"required_fields: record %d is not an object", recordIndex,
				))
				continue
			}
			for _, rawField := range required {
				field, ok := rawField.(string)
				if !ok {
					continue
				}
				if _, found := object[field]; !found {
					violations = append(violations, fmt.Sprintf(
						"required_fields: record %d is missing %q", recordIndex, field,
					))
				}
			}
		}
	}
	return violations
}

func recordValues(value any) ([]any, int64) {
	if values, ok := value.([]any); ok {
		return values, int64(len(values))
	}
	if object, ok := value.(map[string]any); ok {
		if values, exists := object["records"].([]any); exists {
			return values, int64(len(values))
		}
	}
	return []any{value}, 1
}

func integerRule(rules map[string]any, key string) (int64, bool) {
	raw, exists := rules[key]
	if !exists {
		return 0, false
	}
	number, ok := numberRat(raw)
	if !ok || !number.IsInt() || !number.Num().IsInt64() {
		return 0, false
	}
	return number.Num().Int64(), true
}

func isEmptyJSONValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}
