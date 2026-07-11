package protocol

import "encoding/json"

const Port uint32 = 10789

type RunRequest struct {
	NodeKey        string          `json:"node_key"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	Input          json.RawMessage `json:"input"`
}

type RunResult struct {
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	Category   string          `json:"category,omitempty"`
	Retryable  bool            `json:"retryable,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}
