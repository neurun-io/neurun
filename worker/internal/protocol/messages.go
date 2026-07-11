package protocol

import "encoding/json"

const Port uint32 = 10789

type Ready struct {
	Type string `json:"type"`
}

type RunRequest struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Manifest json.RawMessage `json:"manifest"`
	Input    json.RawMessage `json:"input"`
}

type RunResult struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	Category   string          `json:"category,omitempty"`
	Retryable  bool            `json:"retryable,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

type Manifest struct {
	WorkflowRunID  string            `json:"workflow_run_id"`
	NodeKey        string            `json:"node_key"`
	Language       string            `json:"language"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	MemoryLimitMB  int64             `json:"memory_limit_mb"`
	Guest          Paths             `json:"guest"`
	Env            map[string]string `json:"env,omitempty"`
	Command        []string          `json:"command,omitempty"`
}

type Paths struct {
	WorkDir      string `json:"work_dir"`
	CodeDir      string `json:"code_dir"`
	DepsDir      string `json:"deps_dir"`
	InputPath    string `json:"input_path"`
	ManifestPath string `json:"manifest_path"`
}
