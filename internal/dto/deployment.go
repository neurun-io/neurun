// Package dto holds the request and response payloads crossing the API
// boundary, and the mapping from domain records onto them.
//
// Responses are projections, not the domain types themselves: deployment
// Artifact carries StorageKey, the internal blob handle, and serving the domain
// type directly would publish storage topology. Dropping it here is what keeps
// it off the wire.
package dto

import (
	"io"
	"time"

	"github.com/neurun-io/neurun/internal/domain/deployment"
)

type CreateDeploymentRequest struct {
	AppID      string
	Runtime    deployment.Runtime
	EntryPoint string
	SourceName string
	Source     io.Reader
}

type CreateAppRequest struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
}

type UpdateAppRequest struct {
	Name *string `json:"name"`
}

type CreateProjectRequest struct {
	Name string `json:"name"`
}

type UpdateProjectRequest struct {
	Name *string `json:"name"`
}

type ArtifactResponse struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type BuildResponse struct {
	ID           string              `json:"id"`
	ProjectID    string              `json:"project_id"`
	DeploymentID string              `json:"deployment_id"`
	Number       int                 `json:"number"`
	Status       deployment.Status   `json:"status"`
	Runtime      deployment.Runtime  `json:"runtime"`
	EntryPoint   string              `json:"entrypoint"`
	SourceSHA256 string              `json:"source_sha256"`
	Artifacts    []ArtifactResponse  `json:"artifacts"`
	Failure      *deployment.Failure `json:"failure,omitempty"`
	StartedAt    time.Time           `json:"started_at"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
}

type DeploymentResponse struct {
	ID         string             `json:"id"`
	ProjectID  string             `json:"project_id"`
	AppID      string             `json:"app_id"`
	Runtime    deployment.Runtime `json:"runtime"`
	EntryPoint string             `json:"entrypoint"`
	Status     deployment.Status  `json:"status"`
	Source     ArtifactResponse   `json:"source"`
	Builds     []BuildResponse    `json:"builds"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

type ProjectResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AppResponse struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewArtifactResponse(record deployment.Artifact) ArtifactResponse {
	return ArtifactResponse{
		ID: record.ID, Kind: record.Kind, Name: record.Name,
		MediaType: record.MediaType, SizeBytes: record.SizeBytes,
		SHA256: record.SHA256, CreatedAt: record.CreatedAt,
	}
}

func NewBuildResponse(record deployment.Build) BuildResponse {
	artifacts := make([]ArtifactResponse, len(record.Artifacts))
	for index, stored := range record.Artifacts {
		artifacts[index] = NewArtifactResponse(stored)
	}
	return BuildResponse{
		ID: record.ID, ProjectID: record.ProjectID,
		DeploymentID: record.DeploymentID, Number: record.Number,
		Status: record.Status, Runtime: record.Runtime,
		EntryPoint: record.EntryPoint, SourceSHA256: record.SourceSHA256,
		Artifacts: artifacts, Failure: record.Failure,
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt,
	}
}

func NewBuildResponses(records []deployment.Build) []BuildResponse {
	responses := make([]BuildResponse, len(records))
	for index, record := range records {
		responses[index] = NewBuildResponse(record)
	}
	return responses
}

func NewDeploymentResponse(record deployment.Deployment) DeploymentResponse {
	return DeploymentResponse{
		ID: record.ID, ProjectID: record.ProjectID, AppID: record.AppID,
		Runtime: record.Runtime, EntryPoint: record.EntryPoint,
		Status: record.Status, Source: NewArtifactResponse(record.Source),
		Builds:    NewBuildResponses(record.Builds),
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

func NewProjectResponse(record deployment.Project) ProjectResponse {
	return ProjectResponse{
		ID: record.ID, Name: record.Name,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewProjectResponses(records []deployment.Project) []ProjectResponse {
	responses := make([]ProjectResponse, len(records))
	for index, record := range records {
		responses[index] = NewProjectResponse(record)
	}
	return responses
}

func NewAppResponse(record deployment.App) AppResponse {
	return AppResponse{
		ID: record.ID, ProjectID: record.ProjectID, Name: record.Name,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewAppResponses(records []deployment.App) []AppResponse {
	responses := make([]AppResponse, len(records))
	for index, record := range records {
		responses[index] = NewAppResponse(record)
	}
	return responses
}
