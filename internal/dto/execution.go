package dto

import (
	"encoding/json"
	"time"

	"github.com/neurun-io/neurun/internal/domain/execution"
)

// CreateExecutionRequest runs an app. BuildID picks one of its builds; absent
// takes the latest the app has ready, which is what a caller means by "run it".
type CreateExecutionRequest struct {
	AppID   string          `json:"app_id"`
	BuildID string          `json:"build_id"`
	Input   json.RawMessage `json:"input"`
}

type ExecutionResponse struct {
	ID                 string             `json:"id"`
	ProjectID          string             `json:"project_id"`
	DeploymentID       string             `json:"deployment_id"`
	BuildID            string             `json:"build_id"`
	Status             execution.Status   `json:"status"`
	Input              json.RawMessage    `json:"input"`
	Output             json.RawMessage    `json:"output,omitempty"`
	Failure            *execution.Failure `json:"failure,omitempty"`
	Logs               string             `json:"logs"`
	CreatedAt          time.Time          `json:"created_at"`
	StartedAt          *time.Time         `json:"started_at,omitempty"`
	FinishedAt         *time.Time         `json:"finished_at,omitempty"`
	RerunOfExecutionID string             `json:"rerun_of_execution_id,omitempty"`
}

func NewExecutionResponse(record execution.Execution) ExecutionResponse {
	return ExecutionResponse{
		ID: record.ID, ProjectID: record.ProjectID,
		DeploymentID: record.DeploymentID, BuildID: record.BuildID,
		Status: record.Status, Input: record.Input, Output: record.Output,
		Failure: record.Failure, Logs: record.Logs,
		CreatedAt: record.CreatedAt, StartedAt: record.StartedAt,
		FinishedAt:         record.FinishedAt,
		RerunOfExecutionID: record.RerunOfExecutionID,
	}
}

func NewExecutionResponses(records []execution.Execution) []ExecutionResponse {
	responses := make([]ExecutionResponse, len(records))
	for index, record := range records {
		responses[index] = NewExecutionResponse(record)
	}
	return responses
}
