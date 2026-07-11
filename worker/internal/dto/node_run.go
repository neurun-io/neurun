package dto

import (
	"encoding/json"

	"github.com/dagflows/worker/internal/domain"
)

type WorkflowNodeRunRequest struct {
	WorkflowRunID   string                     `json:"workflow_run_id"`
	NodeKey         string                     `json:"node_key"`
	Language        string                     `json:"language"`
	ArtifactKey     string                     `json:"artifact_key"`
	DepsArtifactKey string                     `json:"deps_artifact_key,omitempty"`
	Config          map[string]any             `json:"config,omitempty"`
	MemoryLimitMB   int64                      `json:"memory_limit_mb,omitempty"`
	TimeoutSeconds  int                        `json:"timeout_seconds"`
	InputData       map[string]json.RawMessage `json:"input_data,omitempty"`
}

func (r WorkflowNodeRunRequest) RequiredMemoryMB() int64 {
	if r.MemoryLimitMB > 0 {
		return r.MemoryLimitMB
	}
	switch value := r.Config["memory_limit_mb"].(type) {
	case float64:
		if value > 0 {
			return int64(value)
		}
	case int:
		if value > 0 {
			return int64(value)
		}
	case int64:
		if value > 0 {
			return value
		}
	case json.Number:
		if value, err := value.Int64(); err == nil && value > 0 {
			return value
		}
	}
	return domain.DefaultNodeMemoryMB
}

type WorkflowNodeRunResponse struct {
	WorkflowRunID string                              `json:"workflow_run_id"`
	NodeKey       string                              `json:"node_key"`
	Status        domain.WorkflowNodeRunAttemptStatus `json:"status"`
	RouteTo       []string                            `json:"route_to,omitempty"`
	OutputType    domain.WorkflowNodeOutputType       `json:"output_type"`
	OutputRef     string                              `json:"output_ref,omitempty"`
	OutputSize    int64                               `json:"output_size"`
	InlineOutput  map[string]any                      `json:"inline_output,omitempty"`
	ErrorMessage  string                              `json:"error_message,omitempty"`
	ErrorCategory string                              `json:"error_category,omitempty"`
	Retryable     bool                                `json:"retryable"`
	DurationMs    int                                 `json:"duration_ms"`
}
