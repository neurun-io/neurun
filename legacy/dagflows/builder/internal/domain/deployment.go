package domain

type DeploymentStatus string

const (
	DeploymentStatusSuccess DeploymentStatus = "SUCCESS"
	DeploymentStatusFailed  DeploymentStatus = "FAILED"
)

type DeploymentRequest struct {
	DeploymentID  string `json:"deployment_id"`
	WorkflowID    string `json:"workflow_id"`
	GitURL        string `json:"git_url"`
	GitBranch     string `json:"git_branch,omitempty"`
	GitCommitHash string `json:"git_commit_hash,omitempty"`
}

type DeploymentResponse struct {
	DeploymentID string           `json:"deployment_id"`
	Status       DeploymentStatus `json:"status"`
	ErrorMessage string           `json:"error_message"`
	Nodes        []WorkflowNode   `json:"nodes"`
}

type WorkflowNode struct {
	Key            string         `json:"key"`
	Type           string         `json:"type"`
	Language       string         `json:"language"`
	EntryPoint     string         `json:"entrypoint"`
	ArtifactID     string         `json:"artifact_id"`
	DepsArtifactID string         `json:"deps_artifact_id"`
	Config         map[string]any `json:"config"`
	Depends        []string       `json:"depends"`
	RetryCount     int            `json:"retry_count"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	MemoryLimitMB  int            `json:"memory_limit_mb,omitempty"`
}
