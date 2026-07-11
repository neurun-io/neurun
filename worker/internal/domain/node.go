package domain

const DefaultNodeMemoryMB int64 = 512

type WorkflowNodeRunAttemptStatus string

const (
	WorkflowNodeRunAttemptStatusSuccess WorkflowNodeRunAttemptStatus = "SUCCESS"
	WorkflowNodeRunAttemptStatusFailed  WorkflowNodeRunAttemptStatus = "FAILED"
)

type WorkflowNodeOutputType string

const (
	WorkflowNodeOutputTypeInline    WorkflowNodeOutputType = "INLINE"
	WorkflowNodeOutputTypeReference WorkflowNodeOutputType = "REFERENCE"
)
