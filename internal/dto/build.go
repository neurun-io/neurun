package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
)

// ArtifactResponse drops StorageKey, the internal handle. Serving the domain
// artifact directly would publish storage topology.
type ArtifactResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type BuildResponse struct {
	ID           string             `json:"id"`
	AppID        string             `json:"app_id"`
	DeploymentID string             `json:"deployment_id"`
	Runtime      build.Runtime      `json:"runtime"`
	SourceSHA256 string             `json:"source_sha256"`
	Artifacts    []ArtifactResponse `json:"artifacts"`
	CreatedAt    time.Time          `json:"created_at"`
}

func NewArtifactResponse(record build.Artifact) ArtifactResponse {
	return ArtifactResponse{
		ID: record.ID, Name: record.Name, SizeBytes: record.SizeBytes,
		SHA256: record.SHA256, CreatedAt: record.CreatedAt,
	}
}

func NewBuildResponse(record build.Build) BuildResponse {
	artifacts := make([]ArtifactResponse, len(record.Artifacts))
	for index, stored := range record.Artifacts {
		artifacts[index] = NewArtifactResponse(stored)
	}
	return BuildResponse{
		ID: record.ID, AppID: record.AppID, DeploymentID: record.DeploymentID,
		Runtime: record.Runtime, SourceSHA256: record.SourceSHA256,
		Artifacts: artifacts, CreatedAt: record.CreatedAt,
	}
}

func NewBuildResponses(records []build.Build) []BuildResponse {
	responses := make([]BuildResponse, len(records))
	for index, record := range records {
		responses[index] = NewBuildResponse(record)
	}
	return responses
}
