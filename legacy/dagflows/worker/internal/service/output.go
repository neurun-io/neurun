package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dagflows/worker/internal/domain"
)

type normalizedOutput struct {
	Status        domain.WorkflowNodeRunAttemptStatus
	RouteTo       []string
	OutputType    domain.WorkflowNodeOutputType
	OutputRef     string
	OutputSize    int64
	InlineOutput  map[string]any
	ErrorMessage  string
	ErrorCategory string
	Retryable     bool
}

func normalizeOutput(raw []byte, inlineMaxBytes int64) (normalizedOutput, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return inlineOutput(map[string]any{}, nil, inlineMaxBytes)
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return normalizedOutput{}, fmt.Errorf("runtime output is not JSON: %w", err)
	}

	obj, _ := value.(map[string]any)
	if obj == nil {
		return inlineOutput(map[string]any{"value": value}, nil, inlineMaxBytes)
	}

	if status, ok := stringField(obj, "status"); ok && strings.EqualFold(status, string(domain.WorkflowNodeRunAttemptStatusFailed)) {
		return normalizedOutput{
			Status:        domain.WorkflowNodeRunAttemptStatusFailed,
			ErrorMessage:  firstString(obj, "error_message", "error"),
			ErrorCategory: firstNonEmpty(firstString(obj, "error_category"), "permanent"),
			Retryable:     boolField(obj, "retryable"),
		}, nil
	}

	routeTo := stringSliceField(obj, "route_to")
	if len(routeTo) == 0 {
		routeTo = stringSliceField(obj, "next_nodes")
	}

	if outputType, ok := stringField(obj, "output_type"); ok && strings.EqualFold(outputType, string(domain.WorkflowNodeOutputTypeReference)) {
		outputRef := firstString(obj, "output_ref")
		if outputRef == "" {
			return normalizedOutput{}, fmt.Errorf("reference output requires output_ref")
		}
		return normalizedOutput{
			Status:     domain.WorkflowNodeRunAttemptStatusSuccess,
			RouteTo:    routeTo,
			OutputType: domain.WorkflowNodeOutputTypeReference,
			OutputRef:  outputRef,
			OutputSize: int64Field(obj, "output_size"),
		}, nil
	}

	if inline, ok := mapField(obj, "inline_output"); ok {
		return inlineOutput(inline, routeTo, inlineMaxBytes)
	}
	if output, ok := obj["output"]; ok {
		if outputMap, ok := output.(map[string]any); ok {
			return inlineOutput(outputMap, routeTo, inlineMaxBytes)
		}
		return inlineOutput(map[string]any{"value": output}, routeTo, inlineMaxBytes)
	}

	return inlineOutput(obj, routeTo, inlineMaxBytes)
}

func inlineOutput(value map[string]any, routeTo []string, inlineMaxBytes int64) (normalizedOutput, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return normalizedOutput{}, err
	}
	if inlineMaxBytes > 0 && int64(len(data)) > inlineMaxBytes {
		return normalizedOutput{}, fmt.Errorf("inline output size %d exceeds limit %d; reference outputs need an upload contract", len(data), inlineMaxBytes)
	}
	return normalizedOutput{
		Status:       domain.WorkflowNodeRunAttemptStatusSuccess,
		RouteTo:      routeTo,
		OutputType:   domain.WorkflowNodeOutputTypeInline,
		OutputSize:   int64(len(data)),
		InlineOutput: value,
	}, nil
}

func stringField(obj map[string]any, key string) (string, bool) {
	value, ok := obj[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func firstString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := stringField(obj, key); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func boolField(obj map[string]any, key string) bool {
	value, ok := obj[key]
	if !ok {
		return false
	}
	parsed, ok := value.(bool)
	return ok && parsed
}

func int64Field(obj map[string]any, key string) int64 {
	value, ok := obj[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}

func mapField(obj map[string]any, key string) (map[string]any, bool) {
	value, ok := obj[key]
	if !ok {
		return nil, false
	}
	typed, ok := value.(map[string]any)
	return typed, ok
}

func stringSliceField(obj map[string]any, key string) []string {
	value, ok := obj[key]
	if !ok {
		return nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if ok && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}
