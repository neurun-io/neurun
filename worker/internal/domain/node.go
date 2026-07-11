package domain

const DefaultNodeMemoryMB int64 = 512

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
