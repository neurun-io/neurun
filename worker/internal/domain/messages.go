package domain

import "encoding/json"

type WorkflowNodeRunAttemptStatus string

const (
	WorkflowNodeRunAttemptStatusSuccess WorkflowNodeRunAttemptStatus = "SUCCESS"
	WorkflowNodeRunAttemptStatusFailed  WorkflowNodeRunAttemptStatus = "FAILED"
	WorkflowNodeRunAttemptStatusTimeout WorkflowNodeRunAttemptStatus = "TIMEOUT"
	WorkflowNodeRunAttemptStatusOOM     WorkflowNodeRunAttemptStatus = "OOM"
)

type WorkflowNodeOutputType string

const (
	WorkflowNodeOutputTypeInline    WorkflowNodeOutputType = "INLINE"
	WorkflowNodeOutputTypeReference WorkflowNodeOutputType = "REFERENCE"
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
	TimeoutSeconds  int                        `json:"timeout_seconds"`
	InputData       map[string]json.RawMessage `json:"input_data,omitempty"`
	InputRefs       map[string]string          `json:"input_refs,omitempty"`
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
	WorkflowRunID  string                       `json:"workflow_run_id"`
	NodeKey        string                       `json:"node_key"`
	ExecutionToken int64                        `json:"execution_token"`
	Status         WorkflowNodeRunAttemptStatus `json:"status"`
	RouteTo        []string                     `json:"route_to,omitempty"`
	OutputType     WorkflowNodeOutputType       `json:"output_type"`
	OutputRef      string                       `json:"output_ref,omitempty"`
	OutputSize     int64                        `json:"output_size"`
	InlineOutput   map[string]any               `json:"inline_output,omitempty"`
	ErrorMessage   string                       `json:"error_message,omitempty"`
	ErrorCategory  string                       `json:"error_category,omitempty"`
	Retryable      bool                         `json:"retryable"`
	DurationMs     int                          `json:"duration_ms"`
}
