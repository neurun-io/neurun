package dto

import (
	"encoding/json"

	"github.com/dagflows/worker/internal/domain"
)

type WorkflowNodeRunRequest struct {
	WorkflowRunID   string                     `json:"workflow_run_id"`
	NodeKey         string                     `json:"node_key"`
	ExecutionToken  int64                      `json:"execution_token"`
	Entrypoint      string                     `json:"entrypoint"`
	Language        string                     `json:"language"`
	ArtifactKey     string                     `json:"artifact_key,omitempty"`
	ArtifactURL     string                     `json:"artifact_url"`
	DepsArtifactKey string                     `json:"deps_artifact_key,omitempty"`
	DepsArtifactURL string                     `json:"deps_artifact_url,omitempty"`
	Config          map[string]any             `json:"config,omitempty"`
	MemoryLimitMB   int64                      `json:"memory_limit_mb,omitempty"`
	TimeoutSeconds  int                        `json:"timeout_seconds"`
	InputData       map[string]json.RawMessage `json:"input_data,omitempty"`
	InputRefs       map[string]string          `json:"input_refs,omitempty"`
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

func (r WorkflowNodeRunRequest) CodeArtifactRef() string {
	if r.ArtifactKey != "" {
		return r.ArtifactKey
	}
	return r.ArtifactURL
}

func (r WorkflowNodeRunRequest) DepsArtifactRef() string {
	if r.DepsArtifactKey != "" {
		return r.DepsArtifactKey
	}
	return r.DepsArtifactURL
}

type WorkflowNodeRunResponse struct {
	WorkflowRunID  string                              `json:"workflow_run_id"`
	NodeKey        string                              `json:"node_key"`
	ExecutionToken int64                               `json:"execution_token"`
	Status         domain.WorkflowNodeRunAttemptStatus `json:"status"`
	RouteTo        []string                            `json:"route_to,omitempty"`
	OutputType     domain.WorkflowNodeOutputType       `json:"output_type"`
	OutputRef      string                              `json:"output_ref,omitempty"`
	OutputSize     int64                               `json:"output_size"`
	InlineOutput   map[string]any                      `json:"inline_output,omitempty"`
	ErrorMessage   string                              `json:"error_message,omitempty"`
	ErrorCategory  string                              `json:"error_category,omitempty"`
	Retryable      bool                                `json:"retryable"`
	DurationMs     int                                 `json:"duration_ms"`
}
