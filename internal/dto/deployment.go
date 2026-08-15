// Package dto holds the request and response payloads crossing the API
// boundary, and the mapping from domain records onto them.
//
// Responses are projections, not the domain types themselves: an Artifact
// carries StorageKey, the internal blob handle, and serving the domain type
// directly would publish storage topology. Dropping it here is what keeps it
// off the wire.
package dto

import (
	"io"
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/domain/deployment"
)

type CreateDeploymentRequest struct {
	AppID   string
	Runtime build.Runtime
	// Source is the archive to build. It is read once, into a temporary file the
	// build owns, and never stored.
	Source    io.Reader
	CommitSHA string
	GitRef    string
}

type DeploymentResponse struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	AppID      string            `json:"app_id"`
	Runtime    build.Runtime     `json:"runtime"`
	Status     deployment.Status `json:"status"`
	CommitSHA  string            `json:"commit_sha,omitempty"`
	GitRef     string            `json:"git_ref,omitempty"`
	// Build is what it produced, absent until it has. Logs are what the
	// toolchain printed getting there, and arrive while it still is.
	Build      *BuildResponse      `json:"build,omitempty"`
	Failure    *deployment.Failure `json:"failure,omitempty"`
	Logs       string              `json:"logs"`
	StartedAt  *time.Time          `json:"started_at,omitempty"`
	FinishedAt *time.Time          `json:"finished_at,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

func NewDeploymentResponse(record deployment.Deployment) DeploymentResponse {
	var produced *BuildResponse
	if record.Build != nil {
		output := NewBuildResponse(*record.Build)
		produced = &output
	}
	return DeploymentResponse{
		ID: record.ID, ProjectID: record.ProjectID, AppID: record.AppID,
		Runtime:   record.Runtime,
		Status:    record.Status,
		CommitSHA: record.CommitSHA, GitRef: record.GitRef,
		Build: produced, Failure: record.Failure, Logs: record.Logs,
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewDeploymentResponses(records []deployment.Deployment) []DeploymentResponse {
	responses := make([]DeploymentResponse, len(records))
	for index, record := range records {
		responses[index] = NewDeploymentResponse(record)
	}
	return responses
}
